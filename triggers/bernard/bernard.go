// Package bernard provides an autoscan trigger for Google Drive via the Bernard library.
package bernard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	lowe "github.com/l3uddz/bernard"
	ds "github.com/l3uddz/bernard/datastore"
	"github.com/l3uddz/bernard/datastore/sqlite"
	"github.com/m-rots/stubbs"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

const (
	maxSyncRetries        = 5
	syncWatchdogTimeout   = 30 * time.Minute
	syncSafeSleepDuration = 120 * time.Second
)

// Config holds configuration for the Bernard Google Drive trigger.
type Config struct {
	AccountPath  string             `yaml:"account"`
	CronSchedule string             `yaml:"cron"`
	Priority     int                `yaml:"priority"`
	TimeOffset   time.Duration      `yaml:"time-offset"`
	Verbosity    string             `yaml:"verbosity"`
	Rewrite      []autoscan.Rewrite `yaml:"rewrite"`
	Include      []string           `yaml:"include"`
	Exclude      []string           `yaml:"exclude"`
	Drives       []DriveConfig      `yaml:"drives"`
}

// DriveConfig holds per-drive overrides for the bernard trigger.
type DriveConfig struct {
	ID         string             `yaml:"id"`
	TimeOffset time.Duration      `yaml:"time-offset"`
	Rewrite    []autoscan.Rewrite `yaml:"rewrite"`
	Include    []string           `yaml:"include"`
	Exclude    []string           `yaml:"exclude"`
}

// New creates a Bernard trigger that polls Google Drive for changes using the given config and database.
func New(cfg Config, db *sql.DB) (autoscan.Trigger, error) {
	logger := autoscan.GetLogger(cfg.Verbosity).With().
		Str("trigger", "bernard").
		Logger()

	const scope = "https://www.googleapis.com/auth/drive.readonly"
	auth, err := stubbs.FromFile(cfg.AccountPath, []string{scope})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, autoscan.ErrFatal)
	}

	store, err := sqlite.FromDB(db)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, autoscan.ErrFatal)
	}

	limiter, err := getRateLimiter(auth.Email())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, autoscan.ErrFatal)
	}

	bernard := lowe.New(auth, store,
		lowe.WithPreRequestHook(func() { limiter.Wait(context.Background()) }),
		lowe.WithSafeSleep(syncSafeSleepDuration))

	var drives []drive
	for _, driveCfg := range cfg.Drives {
		rewriter, err := autoscan.NewRewriter(append(driveCfg.Rewrite, cfg.Rewrite...))
		if err != nil {
			return nil, fmt.Errorf("create drive rewriter: %w", err)
		}

		includes := append(driveCfg.Include, cfg.Include...)
		excludes := append(driveCfg.Exclude, cfg.Exclude...)
		filterer, err := autoscan.NewFilterer(includes, excludes)
		if err != nil {
			return nil, fmt.Errorf("create drive filterer: %w", err)
		}

		scanTime := func() time.Time {
			if driveCfg.TimeOffset.Seconds() > 0 {
				return time.Now().Add(driveCfg.TimeOffset)
			}
			return time.Now().Add(cfg.TimeOffset)
		}

		drives = append(drives, drive{
			ID:       driveCfg.ID,
			Rewriter: rewriter,
			Allowed:  filterer,
			ScanTime: scanTime,
		})
	}

	trigger := func(callback autoscan.ProcessorFunc) {
		dmn := daemon{
			log:          logger,
			callback:     callback,
			cronSchedule: cfg.CronSchedule,
			priority:     cfg.Priority,
			drives:       drives,
			bernard:      bernard,
			store:        &bds{store},
			limiter:      limiter,
		}

		// start job(s)
		if err := dmn.startAutoSync(); err != nil {
			logger.Error().
				Err(err).
				Msg("Cron Init Failed")
			return
		}
	}

	return trigger, nil
}

type drive struct {
	ID       string
	Rewriter autoscan.Rewriter
	Allowed  autoscan.Filterer
	ScanTime func() time.Time
}

type daemon struct {
	callback     autoscan.ProcessorFunc
	cronSchedule string
	priority     int
	drives       []drive
	bernard      *lowe.Bernard
	store        *bds
	log          zerolog.Logger
	limiter      *rateLimiter
}

type syncJob struct {
	log      zerolog.Logger
	attempts int
	errors   []error

	cron  *cron.Cron
	jobID cron.EntryID
	fn    func() error
}

func (s *syncJob) Run() {
	// increase attempt counter
	s.attempts++

	// run job
	err := s.fn()

	// handle job response
	switch {
	case err == nil:
		// job completed successfully
		s.attempts = 0
		s.errors = s.errors[:0]
		return

	case errors.Is(err, lowe.ErrInvalidCredentials), errors.Is(err, ds.ErrDataAnomaly), errors.Is(err, lowe.ErrNetwork):
		// retryable error occurred
		s.log.Debug().
			Err(err).
			Int("attempts", s.attempts).
			Msg("Sync Retry")
		s.errors = append(s.errors, err)

	case errors.Is(err, autoscan.ErrFatal):
		// fatal error occurred, we cannot recover from this safely
		s.log.Error().
			Err(err).
			Msg("Sync Fatal")

		s.cron.Remove(s.jobID)
		return

	default:
		// an un-expected/un-handled error occurred, this should be retryable with the same retry logic
		s.log.Warn().
			Err(err).
			Int("attempts", s.attempts).
			Msg("Sync Unexpected Error")
		s.errors = append(s.errors, err)
	}

	// abort if max retries reached
	if s.attempts >= maxSyncRetries {
		s.log.Error().
			Errs("error", s.errors).
			Int("attempts", s.attempts).
			Msg("Sync Stopped")

		s.cron.Remove(s.jobID)
	}
}

func newSyncJob(c *cron.Cron, log zerolog.Logger, job func() error) *syncJob {
	return &syncJob{
		log:      log,
		attempts: 0,
		errors:   make([]error, 0),
		cron:     c,
		fn:       job,
	}
}

// runSyncWithWatchdog runs syncFn and logs a warning if it exceeds the watchdog timeout.
// It always blocks until syncFn completes — never orphans the goroutine.
func (d daemon) runSyncWithWatchdog(driveID string, syncFn func() error) error {
	done := make(chan error, 1)
	go func() { done <- syncFn() }()

	select {
	case err := <-done:
		return err
	case <-time.After(syncWatchdogTimeout):
		d.log.Warn().Str("drive_id", driveID).
			Dur("elapsed", syncWatchdogTimeout).
			Msg("Sync Watchdog")
		return <-done
	}
}

func (d daemon) startAutoSync() error {
	cronInst := cron.New()

	for _, drive := range d.drives {
		if err := d.setupDrive(cronInst, drive); err != nil {
			return err
		}
	}

	cronInst.Start()
	return nil
}

// setupDrive configures a single drive: determines whether a full sync is
// needed, creates the cron job closure, and registers it with cronInst.
func (d daemon) setupDrive(cronInst *cron.Cron, drive drive) error {
	fullSync := false
	driveLogger := d.withDriveLog(drive.ID)

	// full sync required?
	_, err := d.store.PageToken(drive.ID)
	switch {
	case errors.Is(err, ds.ErrFullSync):
		fullSync = true
	case err != nil:
		return fmt.Errorf("%v: determining if full sync required: %w: %w",
			drive.ID, err, autoscan.ErrFatal)
	default:
		// no error, incremental sync
	}

	// create job
	job := newSyncJob(cronInst, driveLogger, func() error {
		// acquire lock
		if acquireErr := d.limiter.Acquire(context.Background(), 1); acquireErr != nil {
			return fmt.Errorf("%v: acquiring sync semaphore: %w: %w",
				drive.ID, acquireErr, autoscan.ErrFatal)
		}
		defer d.limiter.Release(1)

		// full sync
		if fullSync {
			return d.runSyncWithWatchdog(drive.ID, func() error {
				driveLogger.Info().Msg("Full Sync Starting")
				start := time.Now()

				if syncErr := d.bernard.FullSync(drive.ID); syncErr != nil {
					return fmt.Errorf("%v: performing full sync: %w", drive.ID, syncErr)
				}

				driveLogger.Info().Dur("elapsed", time.Since(start)).Msg("Full Sync Finished")
				fullSync = false
				return nil
			})
		}

		// partial sync
		return d.runPartialSync(drive, driveLogger)
	})

	id, err := cronInst.AddJob(d.cronSchedule, cron.NewChain(cron.SkipIfStillRunning(cron.DiscardLogger)).Then(job))
	if err != nil {
		return fmt.Errorf("%v: creating auto sync job for drive: %w", drive.ID, err)
	}

	job.jobID = id
	return nil
}

// runPartialSync runs one incremental sync for the given drive: fetches
// differences via hooks, computes scan paths, and enqueues them.
func (d daemon) runPartialSync(drive drive, driveLogger zerolog.Logger) error {
	dh, diff := d.store.NewDifferencesHook()
	ph := NewPostProcessBernardDiff(drive.ID, d.store, diff)
	ch, paths := NewPathsHook(drive.ID, d.store, diff)

	return d.runSyncWithWatchdog(drive.ID, func() error {
		driveLogger.Debug().Msg("Partial Sync Starting")
		start := time.Now()

		// do partial sync
		syncErr := d.bernard.PartialSync(drive.ID, dh, ph, ch)
		if syncErr != nil {
			return fmt.Errorf("%v: performing partial sync: %w", drive.ID, syncErr)
		}

		driveLogger.Debug().
			Int("new", len(paths.NewFolders)).
			Int("old", len(paths.OldFolders)).
			Dur("elapsed", time.Since(start)).
			Msg("Partial Sync Finished")

		// translate paths to scan task
		task := d.getScanTask(&drive, paths)

		// move scans to processor
		if len(task.scans) > 0 {
			driveLogger.Trace().
				Interface("scans", task.scans).
				Msg("Scans Enqueuing")

			callbackErr := d.callback(task.scans...)
			if callbackErr != nil {
				return fmt.Errorf("%v: moving scans to processor: %w: %w",
					drive.ID, callbackErr, autoscan.ErrFatal)
			}

			driveLogger.Info().
				Int("added", task.added).
				Int("removed", task.removed).
				Msg("Scans Enqueued")
		}

		return nil
	})
}

type scanTask struct {
	scans   []autoscan.Scan
	added   int
	removed int
}

func (d daemon) getScanTask(drive *drive, paths *Paths) *scanTask {
	pathMap := make(map[string]int)
	task := &scanTask{
		scans:   make([]autoscan.Scan, 0),
		added:   0,
		removed: 0,
	}

	for _, p := range paths.NewFolders {
		// rewrite path
		rewritten := drive.Rewriter(p)

		// check if path already seen
		if _, ok := pathMap[rewritten]; ok {
			// already a scan task present
			continue
		} else {
			pathMap[rewritten] = 1
		}

		// is this path allowed?
		if !drive.Allowed(rewritten) {
			continue
		}

		// add scan task
		task.scans = append(task.scans, autoscan.Scan{
			Folder:   filepath.Clean(rewritten),
			Priority: d.priority,
			Time:     drive.ScanTime().Unix(),
		})

		task.added++
	}

	for _, p := range paths.OldFolders {
		// rewrite path
		rewritten := drive.Rewriter(p)

		// check if path already seen
		if _, ok := pathMap[rewritten]; ok {
			// already a scan task present
			continue
		} else {
			pathMap[rewritten] = 1
		}

		// is this path allowed?
		if !drive.Allowed(rewritten) {
			continue
		}

		// add scan task
		task.scans = append(task.scans, autoscan.Scan{
			Folder:   filepath.Clean(rewritten),
			Priority: d.priority,
			Time:     drive.ScanTime().Unix(),
		})

		task.removed++
	}

	return task
}

func (d daemon) withDriveLog(driveID string) zerolog.Logger {
	drive, err := d.store.GetDrive(driveID)
	if err != nil {
		return d.log.With().Str("drive_id", driveID).Logger()
	}

	return d.log.With().Str("drive_id", driveID).Str("drive_name", drive.Name).Logger()
}
