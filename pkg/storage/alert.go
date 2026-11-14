package storage

import (
	"context"
	"fmt"
)

// AlertState records whether a given (node, rule) is currently firing and when it
// last notified. It is persisted so that a server restart does not re-fire a
// transition that was already reported, and so the per-rule cooldown survives a
// restart.
type AlertState struct {
	NodeID       string
	Rule         string
	Firing       bool
	LastNotified int64
}

// LoadAlertStates returns every persisted alert state, used once to prime the
// in-memory evaluator cache.
func (s *Store) LoadAlertStates(ctx context.Context) ([]AlertState, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT node_id, rule, firing, last_notified FROM alert_state`)
	if err != nil {
		return nil, fmt.Errorf("load alert states: %w", err)
	}
	defer rows.Close()

	var out []AlertState
	for rows.Next() {
		var a AlertState
		var firing int
		if err := rows.Scan(&a.NodeID, &a.Rule, &firing, &a.LastNotified); err != nil {
			return nil, fmt.Errorf("load alert states scan: %w", err)
		}
		a.Firing = firing != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveAlertState upserts one (node, rule) alert state.
func (s *Store) SaveAlertState(ctx context.Context, a AlertState) error {
	firing := 0
	if a.Firing {
		firing = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO alert_state (node_id, rule, firing, last_notified) VALUES (?, ?, ?, ?)`,
		a.NodeID, a.Rule, firing, a.LastNotified)
	if err != nil {
		return fmt.Errorf("save alert state: %w", err)
	}
	return nil
}
