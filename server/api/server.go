// Package api implements the argus HTTP API: agent ingest, admin
// authentication, node management, and the dashboard read/stream endpoints.
package api

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/terobox/argus/pkg/protocol"
	"github.com/terobox/argus/server/alert"
	"github.com/terobox/argus/server/auth"
	"github.com/terobox/argus/server/storage"
)

// Server serves the argus HTTP API.
type Server struct {
	store  *storage.Store
	auth   *auth.Auth
	alerts *alert.Evaluator
	hub    *hub
	latest *latestStore
	logger *slog.Logger
}

// New returns a Server backed by store and a. alerts may be nil to disable
// alerting. The server is API-only: the dashboard is a separate static site
// served by nginx in production and by Vite during local development.
func New(store *storage.Store, logger *slog.Logger, a *auth.Auth, alerts *alert.Evaluator) *Server {
	return &Server{store: store, auth: a, alerts: alerts, hub: newHub(), latest: newLatestStore(), logger: logger}
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Liveness for load balancers and the deploy health check (no auth).
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Human-readable landing page for anyone opening the API port directly.
	// The exact "{$}" pattern keeps unknown paths at 404.
	mux.HandleFunc("GET /{$}", s.handleRoot)

	// First-run setup: public only while the database has no admin.
	mux.HandleFunc("GET "+protocol.SetupStatusPath, s.auth.HandleSetupStatus)
	mux.HandleFunc("POST "+protocol.SetupPath, s.auth.HandleSetup)

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
	mux.Handle("POST "+protocol.NodeTokenPath, s.auth.RequireAdmin(s.auth.RequireCSRF(http.HandlerFunc(s.handleRotateNodeToken))))

	// Dashboard reads and live stream (admin).
	mux.Handle("GET "+protocol.NodeDetailPath, s.auth.RequireAdmin(http.HandlerFunc(s.handleNodeDetail)))
	mux.Handle("GET "+protocol.SeriesPath, s.auth.RequireAdmin(http.HandlerFunc(s.handleSeries)))
	mux.Handle("GET "+protocol.StreamPath, s.auth.RequireAdmin(http.HandlerFunc(s.handleStream)))

	return mux
}

// maxBodyBytes caps agent ingest payload size to prevent memory exhaustion from
// a misbehaving or malicious agent.
const maxBodyBytes = 1 << 20 // 1 MiB

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	body := io.Reader(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if r.Header.Get("Content-Encoding") == "gzip" {
		// gzip.NewReader must wrap body (which has the size limit), not r.Body directly.
		zr, err := gzip.NewReader(body)
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

	// Fan out the latest sample to live dashboard clients as a pre-formatted
	// SSE frame, so handleStream can write it directly without reformatting.
	if n := len(rep.Samples); n > 0 {
		s.latest.put(rep.NodeID, rep.Samples[n-1])
		if msg, err := json.Marshal(sampleEvent{NodeID: rep.NodeID, Sample: rep.Samples[n-1]}); err == nil {
			s.hub.publish(fmt.Appendf(nil, "event: sample\ndata: %s\n\n", msg))
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}

// handleRoot answers GET / with a short welcome message instead of the
// ServeMux's bare 404, so an operator who opens the API port learns what the
// process is and where the dashboard lives. It does not touch the database.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"message":    "Welcome to Argus",
		"repository": "https://github.com/terobox/argus",
	})
}

// handleHealthz reports liveness for load balancers and the deploy health
// check. It is unauthenticated and deliberately says nothing about the node
// inventory or other internals: it only confirms the process is up and the
// database is reachable, so a release with a broken database fails its health
// check and the deploy tool can roll it back.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// The database is opened with a single connection and a 5s busy timeout,
	// so give the probe the same budget: a transient long write should not make
	// a healthy deploy fail its health check and roll back.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		s.logger.Warn("health check: database unreachable", "err", err)
		httpError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
