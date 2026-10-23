// Package silo provides an autoscan target for Silo's native v2 scan API.
package silo

import (
	"fmt"
	"path"
	"strings"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

// Config supplies the native API URL and an unscoped admin API token for Silo.
type Config struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type target struct {
	roots        []string
	libraryNames map[string]string
	log          zerolog.Logger
	api          apiClient
}

// New creates a target using cfg's native API URL and token. It calls
// GET /api/v2/libraries once at startup and keeps only enabled libraries'
// non-empty roots. A request or response decode failure returns an error.
func New(cfg Config) (autoscan.Target, error) {
	logger := autoscan.GetLogger("").With().Str("target", "silo").Str("url", cfg.URL).Logger()
	api := newAPIClient(cfg.URL, cfg.Token)
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
		for _, root := range lib.Paths {
			if root == "" {
				continue
			}
			if !strings.HasSuffix(root, "/") {
				root += "/"
			}
			roots = append(roots, root)
		}
	}
	logger.Debug().Strs("libraries", roots).Msg("Libraries Retrieved")
	return &target{roots: roots, libraryNames: names, log: logger, api: api}, nil
}

func (t target) Available() error {
	return t.api.available()
}

func (t target) Scan(scan autoscan.Scan) error {
	folder := path.Clean(scan.Folder)
	if !t.matchesLibrary(folder) {
		t.log.Debug().Str("folder", folder).Msg("Library Not Matched")
		return fmt.Errorf("%w: %s", autoscan.ErrLibraryNotMatched, folder)
	}

	logger := t.log.With().Str("path", folder).Logger()
	logger.Debug().Msg("Scan Sending")
	result, err := t.api.scan(folder)
	if err != nil {
		return fmt.Errorf("silo scan %s: %w", folder, err)
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
		if strings.HasPrefix(folder, root) || autoscan.CleanedPathEqual(folder, root) {
			return true
		}
	}
	return false
}
