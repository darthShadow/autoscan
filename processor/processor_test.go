package processor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/stats"
)

func TestPathExistsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "anchor.txt")
	if err := os.WriteFile(file, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !pathExists(file) {
		t.Error("expected pathExists to return true for existing file")
	}
}

func TestPathExistsDirectory(t *testing.T) {
	dir := t.TempDir()

	if !pathExists(dir) {
		t.Error("expected pathExists to return true for existing directory")
	}
}

func TestPathExistsMissing(t *testing.T) {
	if pathExists("/nonexistent/path/that/does/not/exist") {
		t.Error("expected pathExists to return false for missing path")
	}
}

func newTestProcessor(anchors []string) *Processor {
	return &Processor{
		anchors:     anchors,
		anchorState: make(map[string]bool),
		stats:       stats.New(),
	}
}

// mockTarget is a minimal autoscan.Target for testing callTargets.
type mockTarget struct {
	scanFn func(autoscan.Scan) error
}

func (m *mockTarget) Scan(scan autoscan.Scan) error {
	return m.scanFn(scan)
}

func (*mockTarget) Available() error {
	return nil
}

func TestCallTargets(t *testing.T) {
	p := &Processor{}
	scan := autoscan.Scan{Folder: "/media/movies"}

	t.Run("AllMatch", func(t *testing.T) {
		targets := []autoscan.Target{
			&mockTarget{scanFn: func(_ autoscan.Scan) error { return nil }},
			&mockTarget{scanFn: func(_ autoscan.Scan) error { return nil }},
			&mockTarget{scanFn: func(_ autoscan.Scan) error { return nil }},
		}
		if err := p.callTargets(targets, scan); err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	})

	t.Run("AllSkipped", func(t *testing.T) {
		targets := []autoscan.Target{
			&mockTarget{scanFn: func(_ autoscan.Scan) error {
				return fmt.Errorf("%w: /tv", autoscan.ErrLibraryNotMatched)
			}},
			&mockTarget{scanFn: func(_ autoscan.Scan) error {
				return fmt.Errorf("%w: /tv", autoscan.ErrLibraryNotMatched)
			}},
			&mockTarget{scanFn: func(_ autoscan.Scan) error {
				return fmt.Errorf("%w: /tv", autoscan.ErrLibraryNotMatched)
			}},
		}
		// All skipped — scan is consumed, not retried. No error returned.
		if err := p.callTargets(targets, scan); err != nil {
			t.Errorf("expected nil error when all targets skipped, got: %v", err)
		}
	})

	t.Run("MixMatchAndSkip", func(t *testing.T) {
		targets := []autoscan.Target{
			&mockTarget{scanFn: func(_ autoscan.Scan) error { return nil }},
			&mockTarget{scanFn: func(_ autoscan.Scan) error {
				return fmt.Errorf("%w: /tv", autoscan.ErrLibraryNotMatched)
			}},
			&mockTarget{scanFn: func(_ autoscan.Scan) error { return nil }},
		}
		if err := p.callTargets(targets, scan); err != nil {
			t.Errorf("expected nil error for mixed match/skip, got: %v", err)
		}
	})

	t.Run("RealError", func(t *testing.T) {
		targets := []autoscan.Target{
			&mockTarget{scanFn: func(_ autoscan.Scan) error { return nil }},
			&mockTarget{scanFn: func(_ autoscan.Scan) error {
				return errors.New("connection refused")
			}},
		}
		err := p.callTargets(targets, scan)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("expected error to contain 'connection refused', got: %v", err)
		}
	})

	t.Run("RealErrorPlusSkip", func(t *testing.T) {
		targets := []autoscan.Target{
			&mockTarget{scanFn: func(_ autoscan.Scan) error {
				return fmt.Errorf("%w: /movies", autoscan.ErrLibraryNotMatched)
			}},
			&mockTarget{scanFn: func(_ autoscan.Scan) error { return nil }},
			&mockTarget{scanFn: func(_ autoscan.Scan) error {
				return errors.New("timeout")
			}},
		}
		err := p.callTargets(targets, scan)
		if err == nil {
			t.Fatal("expected non-nil error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("expected error to contain 'timeout', got: %v", err)
		}
		if errors.Is(err, autoscan.ErrLibraryNotMatched) {
			t.Error("returned error must not wrap ErrLibraryNotMatched")
		}
	})
}

func TestCheckAnchorsNoAnchors(t *testing.T) {
	p := newTestProcessor(nil)

	if !p.CheckAnchors() {
		t.Error("expected CheckAnchors to return true when no anchors configured")
	}
}

func TestCheckAnchorsAllPresent(t *testing.T) {
	dir := t.TempDir()

	// Create a file anchor
	file := filepath.Join(dir, "anchor.txt")
	if err := os.WriteFile(file, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a directory anchor
	subdir := filepath.Join(dir, "anchor-dir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	p := newTestProcessor([]string{file, subdir})

	if !p.CheckAnchors() {
		t.Error("expected CheckAnchors to return true when all anchors present")
	}
}

func TestCheckAnchorsOneMissing(t *testing.T) {
	dir := t.TempDir()

	file := filepath.Join(dir, "anchor.txt")
	if err := os.WriteFile(file, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(dir, "does-not-exist")

	p := newTestProcessor([]string{file, missing})

	if p.CheckAnchors() {
		t.Error("expected CheckAnchors to return false when one anchor is missing")
	}
}

func TestCheckAnchorsStateTransitions(t *testing.T) {
	dir := t.TempDir()
	anchor := filepath.Join(dir, "anchor.txt")

	// Start with anchor present
	if err := os.WriteFile(anchor, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newTestProcessor([]string{anchor})

	// First call: initialises state, no transition logged. Should return true.
	if !p.CheckAnchors() {
		t.Fatal("expected true on first call with anchor present")
	}
	if !p.anchorState[anchor] {
		t.Fatal("expected anchorState to be true after first call")
	}

	// Remove anchor: state transitions from available to unavailable.
	if err := os.Remove(anchor); err != nil {
		t.Fatal(err)
	}
	if p.CheckAnchors() {
		t.Fatal("expected false after anchor removed")
	}
	if p.anchorState[anchor] {
		t.Fatal("expected anchorState to be false after removal")
	}

	// Still missing: no transition, still false.
	if p.CheckAnchors() {
		t.Fatal("expected false while anchor still missing")
	}

	// Restore anchor: state transitions from unavailable to available.
	if err := os.WriteFile(anchor, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !p.CheckAnchors() {
		t.Fatal("expected true after anchor restored")
	}
	if !p.anchorState[anchor] {
		t.Fatal("expected anchorState to be true after restore")
	}
}
