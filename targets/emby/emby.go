package emby

import (
	"fmt"
	"path"
	"strings"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

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

func New(c Config) (autoscan.Target, error) {
	l := autoscan.GetLogger(c.Verbosity).With().
		Str("target", "emby").
		Str("url", c.URL).
		Logger()

	rewriter, err := autoscan.NewRewriter(c.Rewrite)
	if err != nil {
		return nil, err
	}

	api := newAPIClient(c.URL, c.Token, l)

	libraries, err := api.Libraries()
	if err != nil {
		return nil, err
	}

	l.Debug().
		Interface("libraries", libraries).
		Msg("Libraries Retrieved")

	return &target{
		url:       c.URL,
		token:     c.Token,
		libraries: libraries,

		log:     l,
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

	l := t.log.With().
		Str("path", scanPath).
		Str("library", lib.Name).
		Logger()

	// send scan request
	l.Debug().Msg("Scan Sending")

	if err := t.api.Scan(scanPath); err != nil {
		return err
	}

	l.Info().Msg("Scan Sent")
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
