package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/terobox/argus/pkg/metrics"
	"github.com/terobox/argus/pkg/protocol"
	"github.com/terobox/argus/server/auth"
	"github.com/terobox/argus/server/storage"
)

// nodeIDRe allows only URL-safe characters that are unambiguous in log lines
// and paths. Intentionally excludes slashes, whitespace, and shell metacharacters.
var nodeIDRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// maxSmallBodyBytes caps the JSON body for admin mutations (createNode,
// deleteNode) which carry only short text fields.
const maxSmallBodyBytes = 64 << 10 // 64 KiB

// nodeView is the JSON shape returned by the node list endpoint. It omits the
// token hash so a viewer cannot reconstruct or leak it. Hostname is empty if the
// agent never reported host info.
type nodeView struct {
	NodeID    string   `json:"node_id"`
	Hostname  string   `json:"hostname"`
	FirstSeen int64    `json:"first_seen"`
	LastSeen  int64    `json:"last_seen"`
	CPUCores  int      `json:"cpu_cores"`
	Tags      []string `json:"tags"`
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.ListNodeSummaries(r.Context())
	if err != nil {
		s.logger.Error("list nodes", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]nodeView, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeView{
			NodeID:    n.NodeID,
			Hostname:  n.Hostname,
			FirstSeen: n.FirstSeen,
			LastSeen:  n.LastSeen,
			CPUCores:  n.CPUCores,
			Tags:      n.Tags,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// nodeDetail is the JSON shape returned for a single node. HostInfo is nil if
// the agent never reported host info; LatestSample is nil if it never reported a
// metric sample.
type nodeDetail struct {
	NodeID       string            `json:"node_id"`
	FirstSeen    int64             `json:"first_seen"`
	LastSeen     int64             `json:"last_seen"`
	HostInfo     *metrics.HostInfo `json:"host_info,omitempty"`
	Tags         []string          `json:"tags"`
	LatestSample *metrics.Sample   `json:"latest_sample,omitempty"`
}

// handleNodeDetail returns one node's metadata plus its host info (if reported)
// and its latest sample (so the view has data immediately, before the next live
// ingest).
func (s *Server) handleNodeDetail(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	node, ok, err := s.store.GetNode(r.Context(), nodeID)
	if err != nil {
		s.logger.Error("get node", "node_id", nodeID, "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok {
		httpError(w, http.StatusNotFound, "node not found")
		return
	}

	hi, has, err := s.store.GetNodeInfo(r.Context(), nodeID)
	if err != nil {
		s.logger.Error("get node info", "node_id", nodeID, "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := nodeDetail{NodeID: node.NodeID, FirstSeen: node.FirstSeen, LastSeen: node.LastSeen, Tags: node.Tags}
	if has {
		out.HostInfo = &hi
	}
	if ls, ok, err := s.store.QueryLatest(r.Context(), nodeID); err == nil && ok {
		out.LatestSample = &ls
	}
	writeJSON(w, http.StatusOK, out)
}

type createNodeRequest struct {
	NodeID string   `json:"node_id"`
	Tags   []string `json:"tags"`
}

type createNodeResponse struct {
	NodeID string `json:"node_id"`
	Token  string `json:"token"`
}

// handleCreateNode registers a new agent and returns its token exactly once.
// Only the token's SHA-256 hash is stored, so it can never be shown again.
func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var req createNodeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSmallBodyBytes)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.NodeID == "" || len(req.NodeID) > protocol.MaxNodeIDLen {
		httpError(w, http.StatusBadRequest, "node_id is required and must be <= 128 chars")
		return
	}
	if !nodeIDRe.MatchString(req.NodeID) {
		httpError(w, http.StatusBadRequest, "node_id may only contain letters, digits, dots, underscores, and hyphens")
		return
	}

	token, err := auth.RandomToken(32)
	if err != nil {
		s.logger.Error("generate node token", "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	now := time.Now().Unix()
	// last_seen starts at 0: the node is registered but has not reported yet, so
	// the dashboard must show it as offline until its first sample arrives. Only
	// a successful ingest (TouchNode) moves last_seen forward.
	if err := s.store.UpsertNode(r.Context(), storage.Node{
		NodeID:    req.NodeID,
		TokenHash: storage.HashToken(token),
		FirstSeen: now,
		LastSeen:  0,
		Tags:      normalizeTags(req.Tags),
	}); err != nil {
		s.logger.Error("create node", "node_id", req.NodeID, "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, createNodeResponse{NodeID: req.NodeID, Token: token})
}

// normalizeTags trims whitespace and drops empty entries, so tags round-trip
// cleanly and never store a stray blank label.
func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

type deleteNodeRequest struct {
	NodeID string `json:"node_id"`
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	var req deleteNodeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSmallBodyBytes)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.NodeID == "" {
		httpError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if err := s.store.DeleteNode(r.Context(), req.NodeID); err != nil {
		s.logger.Error("delete node", "node_id", req.NodeID, "err", err)
		httpError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
