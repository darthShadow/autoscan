// Package silo provides an autoscan target for Silo's native v2 scan API.
package silo

import (
	"fmt"
	"os"
	"path"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

// Config supplies the native API URL, an unscoped admin API token, rewrite
// rules from autoscan paths to Silo paths, and the target log level.
type Config struct {
	URL       string             `yaml:"url"`
	Token     string             `yaml:"token"`
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Verbosity string             `yaml:"verbosity"`
}

type target struct {
	roots        []string
	libraryNames map[string]string
	log          zerolog.Logger
	api          apiClient
	rewrite      autoscan.Rewriter
}

// New creates a target using cfg's native API URL and token. It calls
// GET /api/v2/libraries once at startup and keeps every path of every enabled
// library. An invalid rewrite, request, or response decode failure returns an error.
func New(cfg Config) (autoscan.Target, error) {
	logger := autoscan.GetLogger(cfg.Verbosity).With().Str("target", "silo").Str("url", cfg.URL).Logger()
	rewriter, err := autoscan.NewRewriter(cfg.Rewrite)
	if err != nil {
		return nil, fmt.Errorf("create rewriter: %w", err)
	}
	api := newAPIClient(cfg.URL, cfg.Token, logger)
	libraries, err := api.libraries()
	if err != nil {
		return nil, err
	}

	var roots []string
	names := make(map[string]string)
	for _, lib := range libraries {
		if !lib.Enabled {
			continue
		}
		names[lib.ID] = lib.Name
		roots = append(roots, lib.Paths...)
	}
	logger.Debug().Strs("libraries", roots).Msg("Libraries Retrieved")
	return &target{roots: roots, libraryNames: names, log: logger, api: api, rewrite: rewriter}, nil
}

func (t target) Available() error {
	return t.api.available()
}

func (t target) Scan(scan autoscan.Scan) error {
	remote := path.Clean(t.rewrite(scan.Folder))
	if info, err := os.Stat(scan.Folder); err == nil && !info.IsDir() {
		remote = path.Dir(remote)
	}
	if !t.matchesLibrary(remote) {
		t.log.Debug().Str("folder", remote).Msg("Library Not Matched")
		return fmt.Errorf("%w: %s", autoscan.ErrLibraryNotMatched, remote)
	}

	logger := t.log.With().Str("path", remote).Logger()
	logger.Debug().Msg("Scan Sending")
	result, err := t.api.scan(remote)
	if err != nil {
		return fmt.Errorf("silo scan %s: %w", remote, err)
	}
	if result.decodeErr != nil {
		// The request succeeded; decoding affects only the log details.
		logger.Warn().Err(result.decodeErr).Msg("Scan Response Unreadable")
	} else {
		logger.Info().
			Str("library", t.libraryNames[result.response.LibraryID]).
			Str("library-id", result.response.LibraryID).
			Str("mode", result.response.Mode).
			Msg("Scan Accepted")
	}
	return nil
}

func (t target) matchesLibrary(folder string) bool {
	for _, root := range t.roots {
		if autoscan.PathWithinRoot(folder, root) {
			return true
		}
	}
	return false
}
