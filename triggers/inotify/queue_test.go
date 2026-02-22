package inotify

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

func TestQueueProcessesAfterDelay(t *testing.T) {
	var mu sync.Mutex
	var received []autoscan.Scan

	cb := func(scans ...autoscan.Scan) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, scans...)
		return nil
	}

	log := zerolog.Nop()
	q := newQueue(cb, log, 5)

	q.inputs <- "/media/movies/test"

	// Wait for the worker to pick up the input
	time.Sleep(200 * time.Millisecond)

	// Override the scan time to be in the past so process() picks it up
	q.lock.Lock()
	for k := range q.scans {
		q.scans[k] = time.Now().Add(-1 * time.Second)
	}
	q.lock.Unlock()

	// Wait for the next ticker cycle to process
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 scan, got %d", len(received))
	}

	expected := filepath.Clean("/media/movies/test")
	if received[0].Folder != expected {
		t.Errorf("expected folder %q, got %q", expected, received[0].Folder)
	}

	if received[0].Priority != 5 {
		t.Errorf("expected priority 5, got %d", received[0].Priority)
	}
}

func TestQueueWorkerExitsOnChannelClose(t *testing.T) {
	cb := func(scans ...autoscan.Scan) error {
		return nil
	}

	log := zerolog.Nop()

	// Create queue manually to track worker exit
	q := &queue{
		callback: cb,
		log:      log,
		priority: 1,
		inputs:   make(chan string),
		scans:    make(map[string]time.Time),
		lock:     &sync.Mutex{},
	}

	done := make(chan struct{})
	go func() {
		q.worker()
		close(done)
	}()

	// Close the inputs channel
	close(q.inputs)

	// Worker should exit promptly
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit after channel close")
	}
}
