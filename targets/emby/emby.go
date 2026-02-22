package emby

import (
	"fmt"
	"path"
	"strings"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

// Config holds configuration for the Emby target.
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
	api     apiClient
}

// New creates an Emby target from the given Config.
func New(cfg Config) (autoscan.Target, error) {
	logger := autoscan.GetLogger(cfg.Verbosity).With().
		Str("target", "emby").
		Str("url", cfg.URL).
		Logger()

	rewriter, err := autoscan.NewRewriter(cfg.Rewrite)
	if err != nil {
		return nil, fmt.Errorf("create rewriter: %w", err)
	}

	api := newAPIClient(cfg.URL, cfg.Token, logger)

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
	return t.api.Available()
}

func (t target) Scan(scan autoscan.Scan) error {
	// determine library for this scan
	scanFolder := t.rewrite(scan.Folder)

	lib, err := t.getScanLibrary(scanFolder)
	if err != nil {
		t.log.Warn().
			Err(err).
			Msg("Libraries Not Found")

		return nil
	}

	scanPath := scanFolder
	if scan.RelativePath != "" {
		scanPath = path.Join(scanFolder, scan.RelativePath)
	}

	logger := t.log.With().
		Str("path", scanPath).
		Str("library", lib.Name).
		Logger()

	// send scan request
	logger.Debug().Msg("Scan Sending")

	if err := t.api.Scan(scanPath); err != nil {
		return err
	}

	logger.Info().Msg("Scan Sent")
	return nil
}

func (t target) getScanLibrary(folder string) (*library, error) {
	for _, l := range t.libraries {
		if strings.HasPrefix(folder, l.Path) {
			return &l, nil
		}
		// Library root path
		if autoscan.CleanedPathEqual(folder, l.Path) {
			return &l, nil
		}
	}

	return nil, fmt.Errorf("%v: failed determining library", folder)
}
