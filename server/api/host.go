package api

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/terobox/argus/pkg/protocol"
)

// handleHostInfo ingests a host-info report. Like ingest it is node-token
// authenticated (no admin session), and it accepts an optional gzip body.
func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
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

	var rep protocol.HostReport
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

	if err := s.store.UpsertNodeInfo(r.Context(), rep.NodeID, rep.Info, time.Now().Unix()); err != nil {
		s.logger.Error("upsert host info", "node_id", rep.NodeID, "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
