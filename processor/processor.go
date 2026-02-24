package processor

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/internal/sqlite"
	"github.com/cloudbox/autoscan/stats"
)

// Config holds configuration for the Processor.
type Config struct {
	Anchors    []string
	MinimumAge time.Duration
	Stats      *stats.Stats

	Db *sqlite.DB
}

// New creates a Processor from the given Config.
func New(cfg Config) (*Processor, error) {
	store, err := newDatastore(cfg.Db)
	if err != nil {
		return nil, err
	}

	proc := &Processor{
		anchors:     cfg.Anchors,
		minimumAge:  cfg.MinimumAge,
		store:       store,
		stats:       cfg.Stats,
		db:          cfg.Db,
		anchorState: make(map[string]bool),
	}
	return proc, nil
}

// Processor dequeues scans and dispatches them to media server targets.
type Processor struct {
	anchors     []string
	anchorState map[string]bool // tracks per-anchor availability for transition logging
	minimumAge  time.Duration
	store       *datastore
	stats       *stats.Stats
	db          *sqlite.DB
	processMu   sync.Mutex // Protects against concurrent Process() calls
}

// Add enqueues one or more scans for processing.
func (p *Processor) Add(scans ...autoscan.Scan) error {
	p.stats.Received.Add(int64(len(scans)))
	return p.store.Upsert(scans)
}

// ScansRemaining returns the amount of scans remaining
func (p *Processor) ScansRemaining() (int, error) {
	return p.store.GetScansRemaining()
}

// Stats returns the shared stats instance for external counter access.
func (p *Processor) Stats() *stats.Stats {
	return p.stats
}

// CheckAnchors verifies that all configured anchor paths (files or directories)
// exist. Returns true if all anchors are available (or none are configured).
// Logs only on state transitions (available↔unavailable), not every call.
// Must be called from a single goroutine (the scan loop).
func (p *Processor) CheckAnchors() bool {
	if len(p.anchors) == 0 {
		return true
	}

	allAvailable := true
	for _, anchor := range p.anchors {
		available := pathExists(anchor)
		prev, tracked := p.anchorState[anchor]

		if tracked && prev && !available {
			log.Warn().Str("path", anchor).Msg("Anchor Unavailable")
		} else if tracked && !prev && available {
			log.Info().Str("path", anchor).Msg("Anchor Available")
		}

		p.anchorState[anchor] = available
		if !available {
			allAvailable = false
		}
	}

	return allAvailable
}

const processorTimeout = 90 * time.Second

// CheckAvailability checks whether all targets are available.
// If one target is not available, the error will return.
func (*Processor) CheckAvailability(targets []autoscan.Target) error {
	ctx, cancel := context.WithTimeout(context.Background(), processorTimeout)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	for _, target := range targets {
		g.Go(func() error {
			return target.Available()
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("check target availability: %w", err)
	}
	return nil
}

func (*Processor) callTargets(targets []autoscan.Target, scan autoscan.Scan) error {
	ctx, cancel := context.WithTimeout(context.Background(), processorTimeout)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	for _, target := range targets {
		g.Go(func() error {
			return target.Scan(scan)
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("call targets: %w", err)
	}
	return nil
}

// Process picks the next available scan and dispatches it to all targets.
// Callers must call CheckAnchors() before Process() to gate on anchor availability.
func (p *Processor) Process(targets []autoscan.Target) error {
	// Protect against concurrent processing to prevent duplicate scan processing
	p.processMu.Lock()
	defer p.processMu.Unlock()

	scan, err := p.store.GetAvailableScan(p.minimumAge)
	if err != nil {
		return err
	}

	// Fatal or Target Unavailable -> return original error
	err = p.callTargets(targets, scan)
	if err != nil {
		return err
	}

	err = p.store.Delete(scan)
	if err != nil {
		return err
	}

	p.stats.Processed.Add(1)
	return nil
}

// Close closes the database connections
func (p *Processor) Close() error {
	if err := p.db.Close(); err != nil {
		return fmt.Errorf("close processor db: %w", err)
	}
	return nil
}

// pathExists reports whether a filesystem path (file or directory) exists.
var pathExists = func(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
