package buffer

import (
	"testing"

	"github.com/terobox/argus/pkg/metrics"
)

func sample(ts int64) metrics.Sample { return metrics.Sample{TS: ts} }

func tsOf(s []metrics.Sample) []int64 {
	out := make([]int64, len(s))
	for i := range s {
		out[i] = s[i].TS
	}
	return out
}

func TestFIFOAndDrain(t *testing.T) {
	b := New(3)
	b.Push(sample(1))
	b.Push(sample(2))
	b.Push(sample(3))
	if b.Len() != 3 {
		t.Fatalf("Len = %d, want 3", b.Len())
	}

	got := tsOf(b.Drain())
	want := []int64{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("drained %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("drained %v, want %v", got, want)
		}
	}
	if b.Len() != 0 {
		t.Fatalf("Len after drain = %d, want 0", b.Len())
	}
}

func TestOverflowDropsOldest(t *testing.T) {
	b := New(3)
	for _, ts := range []int64{1, 2, 3, 4, 5} {
		b.Push(sample(ts))
	}
	got := tsOf(b.Drain())
	want := []int64{3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("drained %v, want %v (oldest must be dropped)", got, want)
		}
	}
}

func TestDrainEmpty(t *testing.T) {
	b := New(3)
	if got := b.Drain(); got != nil {
		t.Fatalf("drain empty = %v, want nil", got)
	}
}

func TestReuseAfterDrain(t *testing.T) {
	b := New(2)
	b.Push(sample(1))
	b.Push(sample(2))
	b.Drain()
	b.Push(sample(3))
	got := tsOf(b.Drain())
	if len(got) != 1 || got[0] != 3 {
		t.Fatalf("after reuse drained %v, want [3]", got)
	}
}

func TestCapacityOne(t *testing.T) {
	b := New(1)
	b.Push(sample(1))
	b.Push(sample(2))
	got := tsOf(b.Drain())
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("cap-1 drained %v, want [2]", got)
	}
}

func TestPeekDoesNotDrain(t *testing.T) {
	b := New(5)
	b.Push(sample(1))
	b.Push(sample(2))
	b.Push(sample(3))

	got := tsOf(b.Peek())
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("peek %v, want [1 2 3]", got)
	}
	if b.Len() != 3 {
		t.Fatalf("Len after peek = %d, want 3 (peek must not drain)", b.Len())
	}
}

func TestDropBeforeKeepsNewerSamples(t *testing.T) {
	b := New(5)
	for _, ts := range []int64{1, 2, 3} {
		b.Push(sample(ts))
	}
	b.Push(sample(4)) // arrived during the report
	b.DropBefore(2)   // report sent samples 1 and 2

	got := tsOf(b.Peek())
	want := []int64{3, 4}
	if len(got) != len(want) {
		t.Fatalf("peek %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("peek %v, want %v", got, want)
		}
	}
}

func TestDropBeforeOverflowDuringReport(t *testing.T) {
	// Buffer full; a report is in flight; new samples overwrite the oldest
	// during the report. DropBefore must drop only the reported prefix and keep
	// the fresh samples that overwrote it.
	b := New(3)
	for _, ts := range []int64{1, 2, 3} {
		b.Push(sample(ts))
	}
	reported := b.Peek() // [1 2 3]
	b.Push(sample(4))    // overwrites 1
	b.Push(sample(5))    // overwrites 2
	b.DropBefore(reported[len(reported)-1].TS)

	got := tsOf(b.Peek())
	want := []int64{4, 5}
	if len(got) != len(want) {
		t.Fatalf("peek %v, want %v (must not lose samples 4 and 5)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("peek %v, want %v", got, want)
		}
	}
}

func TestDropBeforeBounds(t *testing.T) {
	b := New(3)
	b.Push(sample(10))
	b.Push(sample(20))

	b.DropBefore(0) // nothing matches
	if b.Len() != 2 {
		t.Fatalf("Len after DropBefore(0) = %d, want 2", b.Len())
	}
	b.DropBefore(99) // everything matches
	if b.Len() != 0 {
		t.Fatalf("Len after DropBefore(99) = %d, want 0", b.Len())
	}
	if got := b.Peek(); got != nil {
		t.Fatalf("peek empty = %v, want nil", got)
	}
}
