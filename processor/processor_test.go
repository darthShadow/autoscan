package processor

import (
	"os"
	"path/filepath"
	"testing"

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

func TestCheckAnchorsDirectorySupport(t *testing.T) {
	dir := t.TempDir()
	anchorDir := filepath.Join(dir, "mount-check")
	if err := os.Mkdir(anchorDir, 0o755); err != nil {
		t.Fatal(err)
	}

	p := newTestProcessor([]string{anchorDir})

	if !p.CheckAnchors() {
		t.Error("expected CheckAnchors to return true for directory anchor")
	}

	// Remove directory
	if err := os.Remove(anchorDir); err != nil {
		t.Fatal(err)
	}
	if p.CheckAnchors() {
		t.Error("expected CheckAnchors to return false after directory removed")
	}
}
