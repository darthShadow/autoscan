package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/rs/zerolog/log"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/processor"
	"github.com/cloudbox/autoscan/stats"
)

func scanStats(st *stats.Stats, proc *processor.Processor, interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		snap := st.Snapshot()

		remaining, err := proc.ScansRemaining()
		switch {
		case err == nil:
			log.Info().
				Int("remaining", remaining).
				Int64("received", snap.Received).
				Int64("processed", snap.Processed).
				Int64("retried", snap.Retried).
				Msg("Scan Stats")

			status := fmt.Sprintf(
				"STATUS=remaining: %d | received: %d | processed: %d | retried: %d",
				remaining, snap.Received, snap.Processed, snap.Retried,
			)
			_, _ = daemon.SdNotify(false, status)

		case errors.Is(err, autoscan.ErrFatal):
			log.Error().
				Err(err).
				Msg("Stats Stopped")
			ticker.Stop()
			return

		default:
			log.Error().
				Err(err).
				Msg("Scan Stats Failed")
		}
	}
}
