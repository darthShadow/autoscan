// Package atrain provides an autoscan trigger for A-Train Google Drive webhooks.
package atrain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/hlog"

	"github.com/cloudbox/autoscan"
)

// Drive holds configuration for a single Google Drive to monitor via A-Train.
type Drive struct {
	ID      string             `yaml:"id"`
	Rewrite []autoscan.Rewrite `yaml:"rewrite"`
}

// Config holds configuration for the A-Train trigger.
type Config struct {
	Drives    []Drive            `yaml:"drives"`
	Priority  int                `yaml:"priority"`
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Verbosity string             `yaml:"verbosity"`
}

// Rewriter is a function that rewrites a Google Drive path for a given drive ID and input path.
type Rewriter = func(drive, input string) string

// New creates an autoscan-compatible HTTP Trigger for A-Train webhooks.
func New(cfg Config) (autoscan.HTTPTrigger, error) {
	rewrites := make(map[string]autoscan.Rewriter)
	for _, drive := range cfg.Drives {
		rewriter, err := autoscan.NewRewriter(append(drive.Rewrite, cfg.Rewrite...))
		if err != nil {
			return nil, fmt.Errorf("create drive rewriter: %w", err)
		}

		rewrites[drive.ID] = rewriter
	}

	globalRewriter, err := autoscan.NewRewriter(cfg.Rewrite)
	if err != nil {
		return nil, fmt.Errorf("create global rewriter: %w", err)
	}

	rewriter := func(drive, input string) string {
		driveRewriter, ok := rewrites[drive]
		if !ok {
			return globalRewriter(input)
		}

		return driveRewriter(input)
	}

	trigger := func(callback autoscan.ProcessorFunc) http.Handler {
		return handler{
			callback: callback,
			priority: cfg.Priority,
			rewrite:  rewriter,
		}
	}

	return trigger, nil
}

type handler struct {
	priority int
	rewrite  Rewriter
	callback autoscan.ProcessorFunc
}

type atrainEvent struct {
	Created []string `json:"created"`
	Deleted []string `json:"deleted"`
}

func (h handler) ServeHTTP(writer http.ResponseWriter, r *http.Request) {
	var err error
	rlog := hlog.FromRequest(r)

	drive := chi.URLParam(r, "drive")

	event := new(atrainEvent)
	err = json.NewDecoder(r.Body).Decode(event)
	if err != nil {
		rlog.Error().Err(err).Msg("Request Decode Failed")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	rlog.Trace().Interface("event", event).Msg("Webhook Payload")

	scans := make([]autoscan.Scan, 0)

	for _, path := range event.Created {
		scans = append(scans, autoscan.Scan{
			Folder:   h.rewrite(drive, path),
			Priority: h.priority,
			Time:     now().Unix(),
		})
	}

	for _, path := range event.Deleted {
		scans = append(scans, autoscan.Scan{
			Folder:   h.rewrite(drive, path),
			Priority: h.priority,
			Time:     now().Unix(),
		})
	}

	err = h.callback(scans...)
	if err != nil {
		rlog.Error().Err(err).Msg("Scan Enqueue Failed")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, scan := range scans {
		rlog.Info().Str("path", scan.Folder).Msg("Scan Enqueued")
	}

	writer.WriteHeader(http.StatusOK)
}

var now = time.Now
