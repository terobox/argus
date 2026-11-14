package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Node is a registered agent. TokenHash is the SHA-256 hex digest of the node's
// token (see HashToken); the plaintext token is never stored. Tags are the
// user-assigned grouping labels, stored as a JSON array string.
type Node struct {
	NodeID    string
	TokenHash string
	FirstSeen int64
	LastSeen  int64
	Tags      []string
}

// HashToken returns the SHA-256 hex digest of a node token. Storing only this
// digest means a database leak does not expose usable agent credentials.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// UpsertNode inserts or replaces a node row; it is used by registration.
// Ingest must use TouchNode (not UpsertNode) so it can never alter a node's
// token hash.
func (s *Store) UpsertNode(ctx context.Context, n Node) error {
	tags, err := json.Marshal(n.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO nodes (node_id, token_hash, first_seen, last_seen, tags) VALUES (?, ?, ?, ?, ?)`,
		n.NodeID, n.TokenHash, n.FirstSeen, n.LastSeen, string(tags)); err != nil {
		return fmt.Errorf("upsert node: %w", err)
	}
	return nil
}

// parseTags decodes a node's tags JSON column. Empty, "null", and malformed
// values all yield an empty (non-nil) slice rather than an error, so a
// pre-existing row (or an empty default) never breaks the read path and the API
// always serializes tags as [] rather than null.
func parseTags(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err != nil {
		return []string{}
	}
	return tags
}

// VerifyNodeToken reports whether token matches nodeID's stored hash. An
// unknown node returns false with no error.
func (s *Store) VerifyNodeToken(ctx context.Context, nodeID, token string) (bool, error) {
	var want string
	err := s.db.QueryRowContext(ctx,
		`SELECT token_hash FROM nodes WHERE node_id = ?`, nodeID).Scan(&want)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verify node token: %w", err)
	}
	return want == HashToken(token), nil
}

// TouchNode updates a node's last_seen timestamp after a successful ingest.
func (s *Store) TouchNode(ctx context.Context, nodeID string, ts int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET last_seen = ? WHERE node_id = ?`, ts, nodeID)
	if err != nil {
		return fmt.Errorf("touch node: %w", err)
	}
	return nil
}

// DeleteNode removes a node (revocation). Its agent's next report is rejected
// with 401 because the token hash no longer matches anything. Historical sample
// rows are intentionally left for retention to age out, not deleted here.
func (s *Store) DeleteNode(ctx context.Context, nodeID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE node_id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	// Drop any alert state so a deleted node's alerts are not re-evaluated.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM alert_state WHERE node_id = ?`, nodeID); err != nil {
		return fmt.Errorf("delete node alert state: %w", err)
	}
	return nil
}

// ListNodes returns all registered nodes ordered by node_id.
func (s *Store) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT node_id, token_hash, first_seen, last_seen, tags FROM nodes ORDER BY node_id`)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var n Node
		var tags string
		if err := rows.Scan(&n.NodeID, &n.TokenHash, &n.FirstSeen, &n.LastSeen, &tags); err != nil {
			return nil, fmt.Errorf("list nodes scan: %w", err)
		}
		n.Tags = parseTags(tags)
		out = append(out, n)
	}
	return out, rows.Err()
}
