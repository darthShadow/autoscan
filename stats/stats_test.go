package stats

import (
	"sync"
	"testing"
)

func TestSnapshotZeroValue(t *testing.T) {
	s := New()
	snap := s.Snapshot()

	if snap.Received != 0 {
		t.Errorf("expected Received=0, got %d", snap.Received)
	}
	if snap.Processed != 0 {
		t.Errorf("expected Processed=0, got %d", snap.Processed)
	}
	if snap.Retried != 0 {
		t.Errorf("expected Retried=0, got %d", snap.Retried)
	}
}

func TestIncrementAndSnapshot(t *testing.T) {
	s := New()

	s.Received.Add(10)
	s.Processed.Add(7)
	s.Retried.Add(2)

	snap := s.Snapshot()

	if snap.Received != 10 {
		t.Errorf("expected Received=10, got %d", snap.Received)
	}
	if snap.Processed != 7 {
		t.Errorf("expected Processed=7, got %d", snap.Processed)
	}
	if snap.Retried != 2 {
		t.Errorf("expected Retried=2, got %d", snap.Retried)
	}
}

func TestConcurrentIncrements(t *testing.T) {
	s := New()

	const goroutines = 100
	const incrementsPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // 3 counters

	for range goroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				s.Received.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				s.Processed.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				s.Retried.Add(1)
			}
		}()
	}

	wg.Wait()
	snap := s.Snapshot()

	expected := int64(goroutines * incrementsPerGoroutine)
	if snap.Received != expected {
		t.Errorf("expected Received=%d, got %d", expected, snap.Received)
	}
	if snap.Processed != expected {
		t.Errorf("expected Processed=%d, got %d", expected, snap.Processed)
	}
	if snap.Retried != expected {
		t.Errorf("expected Retried=%d, got %d", expected, snap.Retried)
	}
}
