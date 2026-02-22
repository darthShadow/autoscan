// Package inotify provides an autoscan trigger based on filesystem change notifications.
package inotify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

// Config holds configuration for the inotify trigger.
type Config struct {
	Priority  int                `yaml:"priority"`
	Verbosity string             `yaml:"verbosity"`
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Include   []string           `yaml:"include"`
	Exclude   []string           `yaml:"exclude"`
	Paths     []PathConfig       `yaml:"paths"`
}

// PathConfig holds per-path overrides for the inotify trigger.
type PathConfig struct {
	Path    string             `yaml:"path"`
	Rewrite []autoscan.Rewrite `yaml:"rewrite"`
	Include []string           `yaml:"include"`
	Exclude []string           `yaml:"exclude"`
}

type daemon struct {
	callback autoscan.ProcessorFunc
	paths    []path
	watcher  *fsnotify.Watcher
	queue    *queue
	log      zerolog.Logger
}

type path struct {
	Path     string
	Rewriter autoscan.Rewriter
	Allowed  autoscan.Filterer
}

// New creates an inotify-based autoscan trigger that watches the configured paths.
func New(cfg Config) (autoscan.Trigger, error) {
	logger := autoscan.GetLogger(cfg.Verbosity).With().
		Str("trigger", "inotify").
		Logger()

	var paths []path
	for _, pathConfig := range cfg.Paths {
		rewriter, err := autoscan.NewRewriter(append(pathConfig.Rewrite, cfg.Rewrite...))
		if err != nil {
			return nil, fmt.Errorf("create path rewriter: %w", err)
		}

		includes := append(pathConfig.Include, cfg.Include...)
		excludes := append(pathConfig.Exclude, cfg.Exclude...)
		filterer, err := autoscan.NewFilterer(includes, excludes)
		if err != nil {
			return nil, fmt.Errorf("create path filterer: %w", err)
		}

		paths = append(paths, path{
			Path:     pathConfig.Path,
			Rewriter: rewriter,
			Allowed:  filterer,
		})
	}

	trigger := func(callback autoscan.ProcessorFunc) {
		d := daemon{
			log:      logger,
			callback: callback,
			paths:    paths,
			queue:    newQueue(callback, logger, cfg.Priority),
		}

		// start job(s)
		if err := d.startMonitoring(); err != nil {
			logger.Error().
				Err(err).
				Msg("Jobs Init Failed")
			return
		}
	}

	return trigger, nil
}

func (d *daemon) startMonitoring() error {
	// create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	d.watcher = watcher

	// setup watcher
	for _, p := range d.paths {
		if err := filepath.Walk(p.Path, d.walkFunc); err != nil {
			_ = d.watcher.Close()
			return fmt.Errorf("walk path %s: %w", p.Path, err)
		}
	}

	// start worker
	go d.worker()

	return nil
}

func (d *daemon) walkFunc(path string, fi os.FileInfo, err error) error {
	// handle error
	if err != nil {
		return fmt.Errorf("walk func: %v: %w", path, err)
	}

	// ignore non-directory
	if !fi.Mode().IsDir() {
		return nil
	}

	if err := d.watcher.Add(path); err != nil {
		return fmt.Errorf("watch directory: %v: %w", path, err)
	}

	d.log.Debug().
		Str("path", path).
		Msg("Watch Added")

	return nil
}

func (d *daemon) getPathObject(path string) (*path, error) {
	for _, p := range d.paths {
		if strings.HasPrefix(path, p.Path) {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("path object not found: %v", path)
}

func (d *daemon) worker() {
	// close watcher
	defer func() { _ = d.watcher.Close() }()

	// process events
	for {
		select {
		case event := <-d.watcher.Events:
			// new filesystem event
			d.log.Trace().
				Interface("event", event).
				Msg("FS Event")

			switch {
			case event.Op&fsnotify.Create == fsnotify.Create:
				// create
				fi, err := os.Stat(event.Name)
				if err != nil {
					d.log.Error().
						Err(err).
						Str("path", event.Name).
						Msg("FS Stat Failed")
					continue
				}

				// watch new directories
				if fi.IsDir() {
					if err := filepath.Walk(event.Name, d.walkFunc); err != nil {
						d.log.Error().
							Err(err).
							Str("path", event.Name).
							Msg("Watch Failed")
					}

					continue
				}

			case event.Op&fsnotify.Rename == fsnotify.Rename, event.Op&fsnotify.Remove == fsnotify.Remove:
				// renamed / removed
			default:
				// ignore this event
				continue
			}

			// get path object
			pathObj, err := d.getPathObject(event.Name)
			if err != nil {
				d.log.Error().
					Err(err).
					Str("path", event.Name).
					Msg("Path Match Failed")
				continue
			}

			// rewrite
			rewritten := pathObj.Rewriter(event.Name)

			// filter
			if !pathObj.Allowed(rewritten) {
				continue
			}

			// get directory where path has an extension
			if filepath.Ext(rewritten) != "" {
				// there was most likely a file extension, use the directory
				rewritten = filepath.Dir(rewritten)
			}

			// move to queue
			d.queue.inputs <- rewritten

		case err := <-d.watcher.Errors:
			d.log.Error().
				Err(err).
				Msg("FS Events Failed")
		}
	}
}

type queue struct {
	callback autoscan.ProcessorFunc
	log      zerolog.Logger
	priority int
	inputs   chan string
	scans    map[string]time.Time
	lock     *sync.Mutex
}

func newQueue(cb autoscan.ProcessorFunc, log zerolog.Logger, priority int) *queue {
	scanQueue := &queue{
		callback: cb,
		log:      log,
		priority: priority,
		inputs:   make(chan string),
		scans:    make(map[string]time.Time),
		lock:     &sync.Mutex{},
	}

	go scanQueue.worker()

	return scanQueue
}

func (q *queue) add(path string) {
	// acquire lock
	q.lock.Lock()
	defer q.lock.Unlock()

	// queue scan task
	q.scans[path] = time.Now().Add(10 * time.Second)
}

func (q *queue) worker() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case path, ok := <-q.inputs:
			if !ok {
				return
			}
			q.add(path)
		case <-ticker.C:
			q.process()
		}
	}
}

func (q *queue) process() {
	q.lock.Lock()

	if len(q.scans) == 0 {
		q.lock.Unlock()
		return
	}

	// collect ready scans under lock
	type readyScan struct {
		path string
		scan autoscan.Scan
	}

	var ready []readyScan
	now := time.Now()
	for pathStr, t := range q.scans {
		if now.Before(t) {
			continue
		}
		ready = append(ready, readyScan{
			path: pathStr,
			scan: autoscan.Scan{
				Folder:   filepath.Clean(pathStr),
				Priority: q.priority,
				Time:     now.Unix(),
			},
		})
		delete(q.scans, pathStr)
	}

	q.lock.Unlock()

	// call callbacks outside lock
	for _, r := range ready {
		err := q.callback(r.scan)
		if err != nil {
			q.log.Error().
				Err(err).
				Str("path", r.path).
				Msg("Scan Enqueue Failed")
		} else {
			q.log.Info().
				Str("path", r.path).
				Msg("Scan Enqueued")
		}
	}
}
