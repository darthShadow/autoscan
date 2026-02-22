package plex

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

// Config holds configuration for the Plex target.
type Config struct {
	URL       string             `yaml:"url"`
	Token     string             `yaml:"token"`
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Verbosity string             `yaml:"verbosity"`
}

type target struct {
	url       string
	token     string
	libraries []library

	log     zerolog.Logger
	rewrite autoscan.Rewriter
	api     *apiClient
}

// New creates a Plex target from the given Config.
func New(cfg Config) (autoscan.Target, error) {
	logger := autoscan.GetLogger(cfg.Verbosity).With().
		Str("target", "plex").
		Str("url", cfg.URL).Logger()

	rewriter, err := autoscan.NewRewriter(cfg.Rewrite)
	if err != nil {
		return nil, fmt.Errorf("create rewriter: %w", err)
	}

	api := newAPIClient(cfg.URL, cfg.Token, logger)

	version, err := api.Version()
	if err != nil {
		return nil, err
	}

	logger.Debug().Str("version", version).Msg("Plex Version")
	if !isSupportedVersion(version) {
		return nil, fmt.Errorf("plex running unsupported version %s: %w", version, autoscan.ErrFatal)
	}

	libraries, err := api.Libraries()
	if err != nil {
		return nil, err
	}

	logger.Debug().
		Interface("libraries", libraries).
		Msg("Libraries Retrieved")

	return &target{
		url:       cfg.URL,
		token:     cfg.Token,
		libraries: libraries,

		log:     logger,
		rewrite: rewriter,
		api:     api,
	}, nil
}

func (t target) Available() error {
	_, err := t.api.Version()
	return err
}

func (t target) Scan(scan autoscan.Scan) error {
	// determine library for this scan
	scanFolder := t.rewrite(scan.Folder)

	libs, err := t.getScanLibrary(scanFolder)
	if err != nil {
		t.log.Warn().
			Err(err).
			Msg("Libraries Not Found")

		return nil
	}

	// send scan request
	for _, lib := range libs {
		logger := t.log.With().
			Str("path", scanFolder).
			Str("library", lib.Name).
			Logger()

		logger.Debug().Msg("Scan Sending")

		if err := t.api.Scan(scanFolder, lib.ID); err != nil {
			return err
		}

		logger.Info().Msg("Scan Sent")
	}

	return nil
}

func (t target) getScanLibrary(folder string) ([]library, error) {
	libraries := make([]library, 0)

	for _, l := range t.libraries {
		if strings.HasPrefix(folder, l.Path) {
			libraries = append(libraries, l)
		}
		// Library root path
		if autoscan.CleanedPathEqual(folder, l.Path) {
			libraries = append(libraries, l)
		}
	}

	if len(libraries) == 0 {
		return nil, fmt.Errorf("%v: failed determining libraries", folder)
	}

	return libraries, nil
}

func isSupportedVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}

	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])

	if major >= 2 || (major == 1 && minor >= 20) {
		return true
	}

	return false
}
