package autoscan

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// GetLogger returns a zerolog.Logger configured to the given verbosity level string.
// If verbosity is empty or unparseable, the global logger is returned unchanged.
func GetLogger(verbosity string) zerolog.Logger {
	if verbosity == "" {
		return log.Logger
	}

	level, err := zerolog.ParseLevel(verbosity)
	if err != nil {
		return log.Logger
	}

	return log.Level(level)
}
