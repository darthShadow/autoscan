package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/internal/sqlite"
	"github.com/cloudbox/autoscan/processor"
	"github.com/cloudbox/autoscan/stats"
	ast "github.com/cloudbox/autoscan/targets/autoscan"
	"github.com/cloudbox/autoscan/targets/emby"
	"github.com/cloudbox/autoscan/targets/jellyfin"
	"github.com/cloudbox/autoscan/targets/plex"
	atrain "github.com/cloudbox/autoscan/triggers/a_train"
	"github.com/cloudbox/autoscan/triggers/bernard"
	"github.com/cloudbox/autoscan/triggers/inotify"
	"github.com/cloudbox/autoscan/triggers/lidarr"
	"github.com/cloudbox/autoscan/triggers/manual"
	"github.com/cloudbox/autoscan/triggers/radarr"
	"github.com/cloudbox/autoscan/triggers/readarr"
	"github.com/cloudbox/autoscan/triggers/sonarr"
)

const (
	logMaxSizeMB  = 5
	logMaxAgeDays = 14
	logMaxBackups = 5

	defaultScanDelay = 5 * time.Second
	defaultPort      = 3030

	serverTimeout = 30 * time.Second

	noScansDelay = 15 * time.Second
)

type authConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"` //nolint:gosec // user-provided credential field
}

type triggersConfig struct {
	Manual  manual.Config    `yaml:"manual"`
	ATrain  atrain.Config    `yaml:"a-train"`
	Bernard []bernard.Config `yaml:"bernard"`
	Inotify []inotify.Config `yaml:"inotify"`
	Lidarr  []lidarr.Config  `yaml:"lidarr"`
	Radarr  []radarr.Config  `yaml:"radarr"`
	Readarr []readarr.Config `yaml:"readarr"`
	Sonarr  []sonarr.Config  `yaml:"sonarr"`
}

type targetsConfig struct {
	Autoscan []ast.Config      `yaml:"autoscan"`
	Emby     []emby.Config     `yaml:"emby"`
	Jellyfin []jellyfin.Config `yaml:"jellyfin"`
	Plex     []plex.Config     `yaml:"plex"`
}

type config struct {
	// General configuration
	Host       []string      `yaml:"host"`
	Port       int           `yaml:"port"`
	MinimumAge time.Duration `yaml:"minimum-age"`
	ScanDelay  time.Duration `yaml:"scan-delay"`
	ScanStats  time.Duration `yaml:"scan-stats"`
	Anchors    []string      `yaml:"anchors"`

	// Authentication for autoscan.HTTPTrigger
	Auth authConfig `yaml:"authentication"`

	// autoscan.HTTPTrigger
	Triggers triggersConfig `yaml:"triggers"`

	// autoscan.Target
	Targets targetsConfig `yaml:"targets"`
}

// ready is set to true after autoscan has fully initialised, and is used by the
// health endpoint to distinguish "starting up" from "running".
var ready atomic.Bool

var (
	// release variables
	Version   string
	Timestamp string
	GitCommit string

	// CLI
	cli struct {
		globals

		// flags
		Config    string `type:"path" default:"${config_file}" env:"AUTOSCAN_CONFIG" help:"Config file path"`
		Database  string `type:"path" default:"${database_file}" env:"AUTOSCAN_DATABASE" help:"Database file path"`
		Log       string `type:"path" default:"${log_file}" env:"AUTOSCAN_LOG" help:"Log file path"`
		Verbosity int    `type:"counter" default:"0" short:"v" env:"AUTOSCAN_VERBOSITY" help:"Log level verbosity"`
		LogLevel  string `default:"" env:"AUTOSCAN_LOG_LEVEL" help:"Log level (trace,debug,info,warn,error,fatal)"`
	}
)

type globals struct {
	Version versionFlag `name:"version" help:"Print version information and quit"`
}

type versionFlag string

func (versionFlag) Decode(_ *kong.DecodeContext) error { return nil }
func (versionFlag) IsBool() bool                       { return true }
func (versionFlag) BeforeApply(app *kong.Kong, vars kong.Vars) error { //nolint:unparam // satisfies kong.Hook interface
	fmt.Println(vars["version"])
	app.Exit(0)
	return nil
}

func main() {
	// parse cli
	ctx := kong.Parse(&cli,
		kong.Name("autoscan"),
		kong.Description("Scan media into target media servers"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Summary: true,
			Compact: true,
		}),
		kong.Vars{
			"version":       fmt.Sprintf("%s (%s@%s)", Version, GitCommit, Timestamp),
			"config_file":   filepath.Join(defaultConfigDirectory("autoscan", "config.yml"), "config.yml"),
			"log_file":      filepath.Join(defaultConfigDirectory("autoscan", "config.yml"), "activity.log"),
			"database_file": filepath.Join(defaultConfigDirectory("autoscan", "config.yml"), "autoscan.db"),
		},
	)

	if err := ctx.Validate(); err != nil {
		fmt.Println("Failed parsing cli:", err)
		os.Exit(1)
	}

	// logger
	setupLogger()

	// datastore
	dbCtx := context.Background()
	db, err := sqlite.NewDB(dbCtx, cli.Database)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Datastore Init Failed")
	}

	// config
	cfg := loadConfig()

	// stats + processor
	procStats := stats.New()
	proc := initProcessor(cfg, db, procStats)

	// Check authentication. If no auth -> warn user.
	if cfg.Auth.Username == "" || cfg.Auth.Password == "" {
		log.Warn().Msg("Webhooks Unauthenticated")
	}

	// daemon triggers
	initDaemonTriggers(cfg, db, proc.Add)

	// http triggers
	router := getRouter(cfg, proc)

	startHTTPServers(cfg, router)

	log.Info().
		Int("manual", 1).
		Int("bernard", len(cfg.Triggers.Bernard)).
		Int("inotify", len(cfg.Triggers.Inotify)).
		Int("lidarr", len(cfg.Triggers.Lidarr)).
		Int("radarr", len(cfg.Triggers.Radarr)).
		Int("readarr", len(cfg.Triggers.Readarr)).
		Int("sonarr", len(cfg.Triggers.Sonarr)).
		Msg("Triggers Initialised")

	// targets
	targets := initTargets(cfg)

	log.Info().
		Int("autoscan", len(cfg.Targets.Autoscan)).
		Int("plex", len(cfg.Targets.Plex)).
		Int("emby", len(cfg.Targets.Emby)).
		Int("jellyfin", len(cfg.Targets.Jellyfin)).
		Msg("Targets Initialised")

	// scan stats
	if cfg.ScanStats.Seconds() > 0 {
		go scanStats(procStats, proc, cfg.ScanStats)
	}

	// display initialised banner
	log.Info().
		Str("version", fmt.Sprintf("%s (%s@%s)", Version, GitCommit, Timestamp)).
		Msg("Autoscan Initialised")

	notifyReady(proc)

	// processor
	log.Info().Msg("Processor Started")
	runScanLoop(proc, targets, cfg.ScanDelay)
}

// initProcessor creates and returns the scan processor from config and database.
// Calls log.Fatal on initialisation error.
func initProcessor(cfg config, db *sqlite.DB, procStats *stats.Stats) *processor.Processor {
	proc, err := processor.New(processor.Config{
		Anchors:    cfg.Anchors,
		MinimumAge: cfg.MinimumAge,
		Stats:      procStats,
		Db:         db,
	})
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Processor Init Failed")
	}

	log.Info().
		Stringer("min_age", cfg.MinimumAge).
		Strs("anchors", cfg.Anchors).
		Msg("Processor Initialised")

	return proc
}

// startHTTPServers starts one goroutine per host address that serves the router.
// Calls log.Fatal if any server fails to start.
func startHTTPServers(cfg config, router http.Handler) {
	for _, hostAddr := range cfg.Host {
		go func(host string) {
			addr := host
			if !strings.Contains(addr, ":") {
				addr = fmt.Sprintf("%s:%d", host, cfg.Port)
			}

			log.Info().Str("addr", addr).Msg("Server Starting")
			server := &http.Server{
				Addr:         addr,
				Handler:      router,
				ReadTimeout:  serverTimeout,
				WriteTimeout: serverTimeout,
			}
			if listenErr := server.ListenAndServe(); listenErr != nil {
				log.Fatal().
					Str("addr", addr).
					Err(listenErr).
					Msg("Server Start Failed")
			}
		}(hostAddr)
	}
}

// notifyReady marks the process as ready (sd_notify + ready flag) and installs
// a signal handler that closes the processor and exits on SIGINT/SIGTERM.
func notifyReady(proc *processor.Processor) {
	// TODO: Add WatchdogSec support — send periodic WATCHDOG=1 from the processing loop
	// to auto-restart hung processes.
	ready.Store(true)

	sdOK, err := daemon.SdNotify(false, daemon.SdNotifyReady)
	if err != nil {
		log.Warn().Err(err).Msg("sd_notify Failed")
	} else if sdOK {
		log.Info().Msg("sd_notify Ready Sent")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("Shutdown Signal")
		_ = proc.Close()
		os.Exit(0) //nolint:revive // signal handler must exit the process
	}()
}

// loadConfig reads and decodes the YAML config file, applying defaults.
// Calls log.Fatal on any I/O or decode error.
func loadConfig() config {
	file, err := os.Open(cli.Config)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Config Open Failed")
	}

	// set default values
	cfg := config{
		MinimumAge: 10 * time.Minute,
		ScanDelay:  defaultScanDelay,
		ScanStats:  1 * time.Hour,
		Host:       []string{""},
		Port:       defaultPort,
	}

	decoder := yaml.NewDecoder(file)
	decoder.SetStrict(true)
	err = decoder.Decode(&cfg)
	_ = file.Close()

	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Config Decode Failed")
	}

	return cfg
}

// setupLogger configures the global zerolog logger using the CLI flags.
// Log level is set from --log-level if provided, otherwise from verbosity count.
func setupLogger() {
	logger := log.Output(io.MultiWriter(zerolog.ConsoleWriter{
		TimeFormat: time.Stamp,
		Out:        os.Stderr,
	}, &lumberjack.Logger{
		Filename:   cli.Log,
		MaxSize:    logMaxSizeMB,
		MaxAge:     logMaxAgeDays,
		MaxBackups: logMaxBackups,
	}))

	if cli.LogLevel != "" {
		level, err := zerolog.ParseLevel(cli.LogLevel)
		if err != nil {
			log.Logger = logger.Level(zerolog.InfoLevel)
			log.Fatal().Str("level", cli.LogLevel).Msg("Invalid Log Level")
		}

		log.Logger = logger.Level(level)

		return
	}

	switch {
	case cli.Verbosity == 1:
		log.Logger = logger.Level(zerolog.DebugLevel)
	case cli.Verbosity > 1:
		log.Logger = logger.Level(zerolog.TraceLevel)
	default:
		log.Logger = logger.Level(zerolog.InfoLevel)
	}
}

// initDaemonTriggers starts the bernard and inotify background triggers.
// Calls log.Fatal on any initialisation error.
func initDaemonTriggers(cfg config, db *sqlite.DB, add autoscan.ProcessorFunc) {
	for _, t := range cfg.Triggers.Bernard {
		trigger, err := bernard.New(t, db.RW())
		if err != nil {
			log.Fatal().
				Err(err).
				Str("trigger", "bernard").
				Msg("Trigger Init Failed")
		}

		go trigger(add)
	}

	for _, t := range cfg.Triggers.Inotify {
		trigger, err := inotify.New(t)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("trigger", "inotify").
				Msg("Trigger Init Failed")
		}

		go trigger(add)
	}
}

// initTargets builds the list of scan targets from the config.
// Calls log.Fatal on any initialisation error.
func initTargets(cfg config) []autoscan.Target {
	targetCount := len(cfg.Targets.Autoscan) + len(cfg.Targets.Plex) + len(cfg.Targets.Emby) + len(cfg.Targets.Jellyfin)
	targets := make([]autoscan.Target, 0, targetCount)

	for _, t := range cfg.Targets.Autoscan {
		target, err := ast.New(t)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("target", "autoscan").
				Str("target_url", t.URL).
				Msg("Target Init Failed")
		}

		targets = append(targets, target)
	}

	for _, t := range cfg.Targets.Plex {
		target, err := plex.New(t)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("target", "plex").
				Str("target_url", t.URL).
				Msg("Target Init Failed")
		}

		targets = append(targets, target)
	}

	for _, t := range cfg.Targets.Emby {
		target, err := emby.New(t)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("target", "emby").
				Str("target_url", t.URL).
				Msg("Target Init Failed")
		}

		targets = append(targets, target)
	}

	for _, t := range cfg.Targets.Jellyfin {
		target, err := jellyfin.New(t)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("target", "jellyfin").
				Str("target_url", t.URL).
				Msg("Target Init Failed")
		}

		targets = append(targets, target)
	}

	return targets
}

// runScanLoop runs the main processing loop until the process exits.
// It checks anchor availability and target availability before processing,
// and backs off on transient errors.
func runScanLoop(proc *processor.Processor, targets []autoscan.Target, scanDelay time.Duration) {
	targetsAvailable := false
	targetsSize := len(targets)

	for {
		// exit when no targets setup
		if targetsSize == 0 {
			log.Fatal().Msg("No Targets")
		}

		// anchor availability gate — if mounts are offline, skip everything
		if !proc.CheckAnchors() {
			time.Sleep(noScansDelay)
			continue
		}

		// target availability checker
		if !targetsAvailable {
			err := proc.CheckAvailability(targets)
			switch {
			case err == nil:
				targetsAvailable = true

			case errors.Is(err, autoscan.ErrFatal):
				log.Fatal().Err(err).Msg("Target Check Failed")

			default:
				log.Error().Err(err).Msg("Targets Unavailable")
				time.Sleep(noScansDelay)
				continue
			}
		}

		// process scans
		err := proc.Process(targets)
		switch {
		case err == nil:
			// Sleep scan-delay between successful requests to reduce the load on targets.
			time.Sleep(scanDelay)

		case errors.Is(err, autoscan.ErrNoScans):
			// No scans currently available, let's wait a couple of seconds
			log.Trace().Msg("No Scans Available")
			time.Sleep(noScansDelay)

		case errors.Is(err, autoscan.ErrTargetUnavailable):
			proc.Stats().Retried.Add(1)
			targetsAvailable = false
			log.Error().Err(err).Msg("Targets Unavailable")
			time.Sleep(noScansDelay)

		case errors.Is(err, autoscan.ErrFatal):
			log.Fatal().Err(err).Msg("Processing Failed")

		default:
			// unexpected error
			log.Fatal().Err(err).Msg("Processing Failed")
		}
	}
}
