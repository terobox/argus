// Package buffer implements the agent's fixed-capacity ring of samples. It is
// what makes reporting resilient to network blips: samples are buffered locally
// and drained in batches; a failed push simply leaves them in the buffer for
// the next attempt.
package buffer

import (
	"sync"

	"github.com/terobox/argus/pkg/metrics"
)

// Buffer is a fixed-capacity FIFO ring of samples, safe for concurrent use by
// one producer (the collector) and one consumer (the pusher). Push always
// succeeds; when full, the oldest sample is silently overwritten — after a long
// outage those points are too stale to be worth reporting anyway.
type Buffer struct {
	mu         sync.Mutex
	buf        []metrics.Sample
	head, size int
}

// New returns a Buffer holding up to capacity samples. A non-positive capacity
// is treated as 1.
func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = 1
	}
	return &Buffer{buf: make([]metrics.Sample, capacity)}
}

// Len reports how many samples are currently buffered.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.size
}

// Cap reports the buffer's capacity.
func (b *Buffer) Cap() int { return len(b.buf) }

// Push appends s, overwriting the oldest sample if the buffer is full.
func (b *Buffer) Push(s metrics.Sample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.size == len(b.buf) {
		b.buf[b.head] = s
		b.head = (b.head + 1) % len(b.buf)
		return
	}
	b.buf[(b.head+b.size)%len(b.buf)] = s
	b.size++
}

// Peek returns a fresh copy of all buffered samples in insertion order without
// removing them. It returns nil when empty.
func (b *Buffer) Peek() []metrics.Sample {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.peekLocked()
}

// DropBefore removes all buffered samples whose TS is <= ts. Samples are
// buffered in increasing-TS order (the collector guarantees this), so this
// drops a prefix and keeps any newer sample that arrived while a report was in
// flight. It is how the pusher removes a successfully-reported batch without
// losing fresh samples.
func (b *Buffer) DropBefore(ts int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for n < b.size && b.buf[(b.head+n)%len(b.buf)].TS <= ts {
		n++
	}
	b.head = (b.head + n) % len(b.buf)
	b.size -= n
	if b.size == 0 {
		b.head = 0
	}
}

// Drain returns all buffered samples in insertion order and empties the buffer.
// It returns nil when empty.
func (b *Buffer) Drain() []metrics.Sample {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.peekLocked()
	b.head = 0
	b.size = 0
	return out
}

// peekLocked copies the buffered samples in order; the caller must hold b.mu.
func (b *Buffer) peekLocked() []metrics.Sample {
	if b.size == 0 {
		return nil
	}
	out := make([]metrics.Sample, b.size)
	for i := 0; i < b.size; i++ {
		out[i] = b.buf[(b.head+i)%len(b.buf)]
	}
	return out
}
