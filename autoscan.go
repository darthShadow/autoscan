// Package autoscan provides core types and utilities for the autoscan media scanner.
package autoscan

import (
	"errors"
	"io"
	"net/http"
)

// A Scan is at the core of Autoscan.
// It defines which path to scan and with which (trigger-given) priority.
//
// The Scan is used across Triggers, Targets and the Processor.
type Scan struct {
	Folder       string
	RelativePath string
	Priority     int
	Time         int64 // Unix timestamp
}

// ProcessorFunc is a callback that receives one or more media scans for processing.
type ProcessorFunc func(...Scan) error

// Trigger is a function that runs a background process and calls the given ProcessorFunc when scans are detected.
type Trigger func(ProcessorFunc)

// A HTTPTrigger is a Trigger which does not run in the background,
// and instead returns a http.Handler.
//
// This http.Handler should be added to the autoscan router in cmd/autoscan.
type HTTPTrigger func(ProcessorFunc) http.Handler

// A Target receives a Scan from the Processor and translates the Scan
// into a format understood by the target.
//
//nolint:iface // Target is the core public contract; all target packages implement it externally
type Target interface {
	Scan(Scan) error
	Available() error
}

const maxResponseBodySize = 10 * 1024 * 1024 // 10MB

// limitedReadCloser wraps an io.LimitedReader with the original closer.
type limitedReadCloser struct {
	io.Reader
	io.Closer
}

// LimitReadCloser wraps rc so that at most maxResponseBodySize bytes are read.
// The underlying body is still closed normally.
func LimitReadCloser(rc io.ReadCloser) io.ReadCloser {
	return &limitedReadCloser{
		Reader: io.LimitReader(rc, maxResponseBodySize),
		Closer: rc,
	}
}

var (
	// ErrTargetUnavailable may occur when a Target goes offline
	// or suffers from fatal errors. In this case, the processor
	// will halt operations until the target is back online.
	ErrTargetUnavailable = errors.New("target unavailable")

	// ErrFatal indicates a severe problem related to development.
	ErrFatal = errors.New("fatal error")

	// ErrNoScans is not an error. It only indicates whether the CLI
	// should sleep longer depending on the processor output.
	ErrNoScans = errors.New("no scans currently available")

	// ErrAnchorUnavailable indicates that an Anchor file is
	// not available on the file system. Processing should halt
	// until all anchors are available.
	ErrAnchorUnavailable = errors.New("anchor file is unavailable")
)
