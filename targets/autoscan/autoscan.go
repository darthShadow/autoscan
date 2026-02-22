package autoscan

import (
	"fmt"
	"path"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

// Config holds configuration for the autoscan target.
type Config struct {
	URL       string             `yaml:"url"`
	User      string             `yaml:"username"`
	Pass      string             `yaml:"password"` //nolint:gosec // user-provided credential, not a hardcoded secret
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Verbosity string             `yaml:"verbosity"`
}

type target struct {
	url  string
	user string
	pass string

	log     zerolog.Logger
	rewrite autoscan.Rewriter
	api     apiClient
}

// New creates an autoscan target that proxies scans to another autoscan instance.
func New(cfg Config) (autoscan.Target, error) {
	logger := autoscan.GetLogger(cfg.Verbosity).With().
		Str("target", "autoscan").
		Str("url", cfg.URL).Logger()

	rewriter, err := autoscan.NewRewriter(cfg.Rewrite)
	if err != nil {
		return nil, fmt.Errorf("create rewriter: %w", err)
	}

	return &target{
		url:  cfg.URL,
		user: cfg.User,
		pass: cfg.Pass,

		log:     logger,
		rewrite: rewriter,
		api:     newAPIClient(cfg.URL, cfg.User, cfg.Pass, logger),
	}, nil
}

func (t target) Scan(scan autoscan.Scan) error {
	scanFolder := t.rewrite(scan.Folder)

	scanPath := ""
	if scan.RelativePath != "" {
		scanPath = path.Join(scan.Folder, scan.RelativePath)
	}

	// send scan request
	logger := t.log.With().
		Str("folder", scanFolder).
		Str("path", scanPath).
		Logger()

	logger.Debug().Msg("Scan Sending")

	if err := t.api.Scan(scanFolder, scanPath); err != nil {
		return err
	}

	logger.Info().Msg("Scan Sent")
	return nil
}

func (t target) Available() error {
	return t.api.Available()
}
