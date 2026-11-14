package auth

import (
	"sync"
	"time"
)

// RateLimiter is a sliding-window-per-key limiter for the login endpoint. It is
// in-memory and best-effort: its job is to slow credential stuffing, not to be a
// precise multi-node quota system.
type RateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	hits   map[string][]time.Time
}

// NewRateLimiter returns a limiter allowing at most limit attempts per window
// per key.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		window: window,
		limit:  limit,
		hits:   make(map[string][]time.Time),
	}
}

// Allow reports whether key may attempt now and, if so, records the attempt.
func (r *RateLimiter) Allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-r.window)

	r.mu.Lock()
	defer r.mu.Unlock()

	var kept []time.Time
	for _, t := range r.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= r.limit {
		r.hits[key] = kept
		return false
	}
	r.hits[key] = append(kept, now)
	return true
}

// Sweep drops keys with no activity in the last window; call it periodically to
// bound memory in the face of many distinct client IPs.
func (r *RateLimiter) Sweep() {
	cutoff := time.Now().Add(-r.window)
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, ts := range r.hits {
		recent := false
		for _, t := range ts {
			if t.After(cutoff) {
				recent = true
				break
			}
		}
		if !recent {
			delete(r.hits, k)
		}
	}
}
