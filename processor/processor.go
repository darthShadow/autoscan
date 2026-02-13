package processor

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/internal/sqlite"

	"golang.org/x/sync/errgroup"
)

type Config struct {
	Anchors    []string
	MinimumAge time.Duration

	Db *sqlite.DB
}

func New(c Config) (*Processor, error) {
	store, err := newDatastore(c.Db)
	if err != nil {
		return nil, err
	}

	proc := &Processor{
		anchors:    c.Anchors,
		minimumAge: c.MinimumAge,
		store:      store,
		db:         c.Db,
	}
	return proc, nil
}

type Processor struct {
	anchors    []string
	minimumAge time.Duration
	store      *datastore
	processed  int64
	db         *sqlite.DB
	processMu  sync.Mutex // Protects against concurrent Process() calls
}

func (p *Processor) Add(scans ...autoscan.Scan) error {
	return p.store.Upsert(scans)
}

// ScansRemaining returns the amount of scans remaining
func (p *Processor) ScansRemaining() (int, error) {
	return p.store.GetScansRemaining()
}

// ScansProcessed returns the amount of scans processed
func (p *Processor) ScansProcessed() int64 {
	return atomic.LoadInt64(&p.processed)
}

const processorTimeout = 90 * time.Second

// CheckAvailability checks whether all targets are available.
// If one target is not available, the error will return.
func (p *Processor) CheckAvailability(targets []autoscan.Target) error {
	ctx, cancel := context.WithTimeout(context.Background(), processorTimeout)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	for _, target := range targets {
		g.Go(func() error {
			return target.Available()
		})
	}

	return g.Wait()
}

func (p *Processor) callTargets(targets []autoscan.Target, scan autoscan.Scan) error {
	ctx, cancel := context.WithTimeout(context.Background(), processorTimeout)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	for _, target := range targets {
		g.Go(func() error {
			return target.Scan(scan)
		})
	}

	return g.Wait()
}

func (p *Processor) Process(targets []autoscan.Target) error {
	// Protect against concurrent processing to prevent duplicate scan processing
	p.processMu.Lock()
	defer p.processMu.Unlock()

	scan, err := p.store.GetAvailableScan(p.minimumAge)
	if err != nil {
		return err
	}

	// Check whether all anchors are present
	for _, anchor := range p.anchors {
		if !fileExists(anchor) {
			return fmt.Errorf("%s: %w", anchor, autoscan.ErrAnchorUnavailable)
		}
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

	atomic.AddInt64(&p.processed, 1)
	return nil
}

// Close closes the database connections
func (p *Processor) Close() error {
	return p.db.Close()
}

var fileExists = func(fileName string) bool {
	info, err := os.Stat(fileName)
	if err != nil {
		return false
	}

	return !info.IsDir()
}
