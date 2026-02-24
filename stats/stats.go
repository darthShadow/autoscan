// Package stats provides atomic counters for scan processing metrics.
package stats

import "sync/atomic"

// Stats holds atomic counters for scan processing metrics.
type Stats struct {
	Received  atomic.Int64
	Processed atomic.Int64
	Retried   atomic.Int64
}

// New returns a zero-valued Stats ready for use.
func New() *Stats {
	return &Stats{}
}

// Snapshot is a plain-struct copy of all counters at a point in time.
type Snapshot struct {
	Received  int64
	Processed int64
	Retried   int64
}

// Snapshot reads all counters atomically and returns a plain copy.
func (s *Stats) Snapshot() Snapshot {
	return Snapshot{
		Received:  s.Received.Load(),
		Processed: s.Processed.Load(),
		Retried:   s.Retried.Load(),
	}
}
