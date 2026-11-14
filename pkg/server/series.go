package server

import (
	"net/http"
	"strconv"

	"github.com/terobox/argus/pkg/storage"
)

type seriesResponse struct {
	Resolution string                `json:"resolution"`
	Points     []storage.SeriesPoint `json:"points"`
}

// handleSeries serves GET /api/v1/nodes/{node_id}/series?from=&to=, returning the
// resolution-appropriate metric series. Requires an admin session.
func (s *Server) handleSeries(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	from, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	if err != nil {
		httpError(w, http.StatusBadRequest, "from must be an integer unix timestamp")
		return
	}
	to, err := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err != nil {
		httpError(w, http.StatusBadRequest, "to must be an integer unix timestamp")
		return
	}

	pts, res, err := s.store.QuerySeries(r.Context(), nodeID, from, to)
	if err != nil {
		s.logger.Error("query series", "node_id", nodeID, "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if pts == nil {
		pts = []storage.SeriesPoint{}
	}
	writeJSON(w, http.StatusOK, seriesResponse{Resolution: res, Points: pts})
}
