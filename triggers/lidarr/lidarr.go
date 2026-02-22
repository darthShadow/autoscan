// Package lidarr provides an autoscan trigger for Lidarr webhooks.
package lidarr

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/rs/zerolog/hlog"

	"github.com/cloudbox/autoscan"
)

// Config holds configuration for the Lidarr trigger.
type Config struct {
	Name      string             `yaml:"name"`
	Priority  int                `yaml:"priority"`
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Verbosity string             `yaml:"verbosity"`
}

// New creates an autoscan-compatible HTTP Trigger for Lidarr webhooks.
func New(c Config) (autoscan.HTTPTrigger, error) {
	rewriter, err := autoscan.NewRewriter(c.Rewrite)
	if err != nil {
		return nil, fmt.Errorf("create rewriter: %w", err)
	}

	trigger := func(callback autoscan.ProcessorFunc) http.Handler {
		return handler{
			callback: callback,
			priority: c.Priority,
			rewrite:  rewriter,
		}
	}

	return trigger, nil
}

type handler struct {
	priority int
	rewrite  autoscan.Rewriter
	callback autoscan.ProcessorFunc
}

type lidarrFile struct {
	Path string `json:"path"`
}

type lidarrEvent struct {
	Type    string `json:"eventType"`
	Upgrade bool   `json:"isUpgrade"`

	Files []lidarrFile `json:"trackFiles"`
}

func (h handler) ServeHTTP(writer http.ResponseWriter, r *http.Request) {
	var err error
	logger := hlog.FromRequest(r)

	event := new(lidarrEvent)
	err = json.NewDecoder(r.Body).Decode(event)
	if err != nil {
		logger.Error().Err(err).Msg("Request Decode Failed")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	logger.Trace().Interface("event", event).Msg("Webhook Payload")

	if strings.EqualFold(event.Type, "Test") {
		logger.Info().Msg("Test Event")
		writer.WriteHeader(http.StatusOK)
		return
	}

	if !strings.EqualFold(event.Type, "Download") || len(event.Files) == 0 {
		logger.Error().Msg("Required Fields Missing")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	unique := make(map[string]bool)
	scans := make([]autoscan.Scan, 0)

	for _, f := range event.Files {
		folderPath := path.Dir(h.rewrite(f.Path))
		if _, ok := unique[folderPath]; ok {
			continue
		}

		// add scan
		unique[folderPath] = true
		scans = append(scans, autoscan.Scan{
			Folder:   folderPath,
			Priority: h.priority,
			Time:     now().Unix(),
		})
	}

	err = h.callback(scans...)
	if err != nil {
		logger.Error().Err(err).Msg("Scan Enqueue Failed")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.WriteHeader(http.StatusOK)
	logger.Info().
		Str("path", scans[0].Folder).
		Str("event", event.Type).
		Msg("Scan Enqueued")
}

var now = time.Now
