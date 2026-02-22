// Package radarr provides an autoscan trigger for Radarr webhooks.
package radarr

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

// Config holds configuration for the Radarr trigger.
type Config struct {
	Name      string             `yaml:"name"`
	Priority  int                `yaml:"priority"`
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Verbosity string             `yaml:"verbosity"`
}

// New creates an autoscan-compatible HTTP Trigger for Radarr webhooks.
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

type radarrFile struct {
	RelativePath string `json:"relativePath"`
}

type radarrMovie struct {
	FolderPath string `json:"folderPath"`
}

type radarrEvent struct {
	Type  string      `json:"eventType"`
	File  radarrFile  `json:"movieFile"`
	Movie radarrMovie `json:"movie"`
}

func (h handler) ServeHTTP(writer http.ResponseWriter, r *http.Request) {
	var err error
	rlog := hlog.FromRequest(r)

	event := new(radarrEvent)
	err = json.NewDecoder(r.Body).Decode(event)
	if err != nil {
		rlog.Error().Err(err).Msg("Request Decode Failed")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	rlog.Trace().Interface("event", event).Msg("Webhook Payload")

	if strings.EqualFold(event.Type, "Test") {
		rlog.Info().Msg("Test Event")
		writer.WriteHeader(http.StatusOK)
		return
	}

	var (
		folderPath string
		filePath   string
	)

	if strings.EqualFold(event.Type, "Download") || strings.EqualFold(event.Type, "MovieFileDelete") {
		if event.File.RelativePath == "" || event.Movie.FolderPath == "" {
			rlog.Error().Msg("Required Fields Missing")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		folderPath = path.Dir(path.Join(event.Movie.FolderPath, event.File.RelativePath))
		filePath = path.Base(path.Join(event.Movie.FolderPath, event.File.RelativePath))
	}

	if strings.EqualFold(event.Type, "MovieDelete") || strings.EqualFold(event.Type, "Rename") {
		if event.Movie.FolderPath == "" {
			rlog.Error().Msg("Required Fields Missing")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		folderPath = event.Movie.FolderPath
	}

	scan := autoscan.Scan{
		Folder:       h.rewrite(folderPath),
		RelativePath: filePath,
		Priority:     h.priority,
		Time:         now().Unix(),
	}

	err = h.callback(scan)
	if err != nil {
		rlog.Error().Err(err).Msg("Scan Enqueue Failed")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	rlog.Info().
		Str("path", folderPath).
		Str("event", event.Type).
		Msg("Scan Enqueued")

	writer.WriteHeader(http.StatusOK)
}

var now = time.Now
