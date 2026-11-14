// Package server implements the argus HTTP API: agent ingest, admin
// authentication, node management, and the dashboard read/stream endpoints.
package server

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/terobox/argus/pkg/alert"
	"github.com/terobox/argus/pkg/auth"
	"github.com/terobox/argus/pkg/protocol"
	"github.com/terobox/argus/pkg/storage"
	"github.com/terobox/argus/web"
)

// Server serves the argus HTTP API.
type Server struct {
	store  *storage.Store
	auth   *auth.Auth
	alerts *alert.Evaluator
	hub    *hub
	ui     http.Handler
	logger *slog.Logger
}

// New returns a Server backed by store and a. alerts may be nil to disable
// alerting.
func New(store *storage.Store, logger *slog.Logger, a *auth.Auth, alerts *alert.Evaluator) *Server {
	dist, _ := web.Dist()
	return &Server{store: store, auth: a, alerts: alerts, hub: newHub(), ui: newUIHandler(dist), logger: logger}
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Agent ingest.
	mux.HandleFunc("POST "+protocol.IngestPath, s.handleReport)
	mux.HandleFunc("POST "+protocol.HostPath, s.handleHostInfo)

	// Auth.
	mux.Handle("POST "+protocol.LoginPath, http.HandlerFunc(s.auth.HandleLogin))
	mux.Handle("POST "+protocol.LogoutPath, http.HandlerFunc(s.auth.HandleLogout))
	mux.Handle("GET "+protocol.MePath, s.auth.RequireAdmin(http.HandlerFunc(s.auth.HandleMe)))

	// Node management (admin; mutations also require CSRF).
	mux.Handle("GET "+protocol.NodesPath, s.auth.RequireAdmin(http.HandlerFunc(s.handleListNodes)))
	mux.Handle("POST "+protocol.NodesPath, s.auth.RequireAdmin(s.auth.RequireCSRF(http.HandlerFunc(s.handleCreateNode))))
	mux.Handle("DELETE "+protocol.NodesPath, s.auth.RequireAdmin(s.auth.RequireCSRF(http.HandlerFunc(s.handleDeleteNode))))

	// Dashboard reads and live stream (admin).
	mux.Handle("GET "+protocol.NodeDetailPath, s.auth.RequireAdmin(http.HandlerFunc(s.handleNodeDetail)))
	mux.Handle("GET "+protocol.SeriesPath, s.auth.RequireAdmin(http.HandlerFunc(s.handleSeries)))
	mux.Handle("GET "+protocol.StreamPath, s.auth.RequireAdmin(http.HandlerFunc(s.handleStream)))

	// Embedded web UI (SPA fallback) as the catch-all.
	mux.Handle("/", s.ui)

	return mux
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	body := io.Reader(r.Body)
	if r.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			httpError(w, http.StatusBadRequest, "invalid gzip body")
			return
		}
		defer zr.Close()
		body = zr
	}

	var rep protocol.Report
	if err := json.NewDecoder(body).Decode(&rep); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := rep.Validate(); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	ok, err := s.store.VerifyNodeToken(r.Context(), rep.NodeID, r.Header.Get(protocol.NodeTokenHeader))
	if err != nil {
		s.logger.Error("verify node token", "node_id", rep.NodeID, "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok {
		httpError(w, http.StatusUnauthorized, "invalid node token")
		return
	}

	if _, err := s.store.WriteSamples(r.Context(), rep.NodeID, rep.Samples); err != nil {
		s.logger.Error("write samples", "node_id", rep.NodeID, "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// last_seen is best-effort: a failure here must not fail the ingest.
	if err := s.store.TouchNode(r.Context(), rep.NodeID, rep.CollectedAt); err != nil {
		s.logger.Warn("touch node", "node_id", rep.NodeID, "err", err)
	}

	// Fan out the latest sample to live dashboard clients.
	if n := len(rep.Samples); n > 0 {
		if msg, err := json.Marshal(sampleEvent{NodeID: rep.NodeID, Sample: rep.Samples[n-1]}); err == nil {
			s.hub.publish(msg)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}
