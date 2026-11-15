package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/terobox/argus/pkg/metrics"
)

// UpsertNodeInfo stores a host's identifying information for nodeID, replacing
// any previous row. updatedAt is a unix timestamp used for staleness checks.
func (s *Store) UpsertNodeInfo(ctx context.Context, nodeID string, hi metrics.HostInfo, updatedAt int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO node_info
		(node_id, hostname, os, kernel, arch, cpu_model, cpu_cores, mem_total, public_ip, agent_ver, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nodeID, hi.Hostname, hi.OS, hi.Kernel, hi.Arch, hi.CPUModel, hi.CPUCores, hi.MemTotal, hi.PublicIP, hi.AgentVer, updatedAt)
	if err != nil {
		return fmt.Errorf("upsert node info: %w", err)
	}
	return nil
}

// GetNodeInfo returns the stored host info for nodeID, or ok=false if absent.
func (s *Store) GetNodeInfo(ctx context.Context, nodeID string) (metrics.HostInfo, bool, error) {
	var hi metrics.HostInfo
	err := s.db.QueryRowContext(ctx, `
		SELECT hostname, os, kernel, arch, cpu_model, cpu_cores, mem_total, public_ip, agent_ver
		FROM node_info WHERE node_id = ?`, nodeID).
		Scan(&hi.Hostname, &hi.OS, &hi.Kernel, &hi.Arch, &hi.CPUModel, &hi.CPUCores, &hi.MemTotal, &hi.PublicIP, &hi.AgentVer)
	if err == sql.ErrNoRows {
		return metrics.HostInfo{}, false, nil
	}
	if err != nil {
		return metrics.HostInfo{}, false, fmt.Errorf("get node info: %w", err)
	}
	return hi, true, nil
}

// NodeSummary is a node row plus its hostname, core count, and tags (empty if
// the agent never reported host info). It omits the token hash.
type NodeSummary struct {
	NodeID    string
	Hostname  string
	FirstSeen int64
	LastSeen  int64
	CPUCores  int
	Tags      []string
}

// ListNodeSummaries returns all nodes with their hostname and core count,
// ordered by node_id.
func (s *Store) ListNodeSummaries(ctx context.Context) ([]NodeSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.node_id, COALESCE(i.hostname, ''), n.first_seen, n.last_seen,
		       COALESCE(i.cpu_cores, 0), n.tags
		FROM nodes n LEFT JOIN node_info i ON i.node_id = n.node_id
		ORDER BY n.node_id`)
	if err != nil {
		return nil, fmt.Errorf("list node summaries: %w", err)
	}
	defer rows.Close()

	var out []NodeSummary
	for rows.Next() {
		var n NodeSummary
		var tags string
		if err := rows.Scan(&n.NodeID, &n.Hostname, &n.FirstSeen, &n.LastSeen, &n.CPUCores, &tags); err != nil {
			return nil, fmt.Errorf("list node summaries scan: %w", err)
		}
		n.Tags = parseTags(tags)
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetNode returns the node named nodeID, or ok=false if absent.
func (s *Store) GetNode(ctx context.Context, nodeID string) (Node, bool, error) {
	var n Node
	var tags string
	err := s.db.QueryRowContext(ctx,
		`SELECT node_id, token_hash, first_seen, last_seen, tags FROM nodes WHERE node_id = ?`, nodeID).
		Scan(&n.NodeID, &n.TokenHash, &n.FirstSeen, &n.LastSeen, &tags)
	if err == sql.ErrNoRows {
		return Node{}, false, nil
	}
	if err != nil {
		return Node{}, false, fmt.Errorf("get node: %w", err)
	}
	n.Tags = parseTags(tags)
	return n, true, nil
}
