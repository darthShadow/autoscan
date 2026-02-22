// Package sonarr provides an autoscan trigger for Sonarr webhooks.
package sonarr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/rs/zerolog/hlog"

	"github.com/cloudbox/autoscan"
)

// Config holds configuration for the Sonarr trigger.
type Config struct {
	Name      string             `yaml:"name"`
	Priority  int                `yaml:"priority"`
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Verbosity string             `yaml:"verbosity"`
}

// New creates an autoscan-compatible HTTP Trigger for Sonarr webhooks.
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

type sonarrFile struct {
	RelativePath string `json:"relativePath"`
}

type sonarrSeries struct {
	Path string `json:"path"`
}

type sonarrRenamedFile struct {
	// use PreviousPath as the Series.Path might have changed.
	PreviousPath string `json:"previousPath"`
	RelativePath string `json:"relativePath"`
}

type sonarrEvent struct {
	Type         string              `json:"eventType"`
	File         sonarrFile          `json:"episodeFile"`
	Series       sonarrSeries        `json:"series"`
	RenamedFiles []sonarrRenamedFile `json:"renamedEpisodeFiles"`
}

func (h handler) ServeHTTP(writer http.ResponseWriter, r *http.Request) {
	rlog := hlog.FromRequest(r)

	event := new(sonarrEvent)
	if err := json.NewDecoder(r.Body).Decode(event); err != nil {
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
		paths map[string]string
		err   error
	)

	switch {
	case strings.EqualFold(event.Type, "Download") || strings.EqualFold(event.Type, "EpisodeFileDelete"):
		paths, err = pathsForDownload(event)
	case strings.EqualFold(event.Type, "SeriesDelete"):
		paths, err = pathsForSeriesDelete(event)
	case strings.EqualFold(event.Type, "Rename"):
		paths, err = pathsForRename(event)
	default:
		// unknown event type — nothing to scan
	}

	if err != nil {
		rlog.Error().Err(err).Msg("Required Fields Missing")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	var scans []autoscan.Scan

	for folderPath, filePath := range paths {
		scans = append(scans, autoscan.Scan{
			Folder:       h.rewrite(folderPath),
			RelativePath: filePath,
			Priority:     h.priority,
			Time:         now().Unix(),
		})
	}

	if err = h.callback(scans...); err != nil {
		rlog.Error().Err(err).Msg("Scan Enqueue Failed")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, scan := range scans {
		rlog.Info().
			Str("path", scan.Folder).
			Str("event", event.Type).
			Msg("Scan Enqueued")
	}

	writer.WriteHeader(http.StatusOK)
}

// pathsForDownload returns the folder→file mapping for Download and EpisodeFileDelete events.
// A Download event covers both new files and upgrades.
func pathsForDownload(event *sonarrEvent) (map[string]string, error) {
	if event.File.RelativePath == "" || event.Series.Path == "" {
		return nil, errors.New("required fields missing")
	}

	// Use path.Dir to get the directory in which the file is located.
	// Use path.Base to get the filename.
	full := path.Join(event.Series.Path, event.File.RelativePath)
	return map[string]string{path.Dir(full): path.Base(full)}, nil
}

// pathsForSeriesDelete returns the series root folder for a SeriesDelete event.
func pathsForSeriesDelete(event *sonarrEvent) (map[string]string, error) {
	if event.Series.Path == "" {
		return nil, errors.New("required fields missing")
	}

	return map[string]string{event.Series.Path: ""}, nil
}

// pathsForRename returns all affected folder→file mappings for a Rename event.
// Both previous and current paths are included; duplicates are dropped.
func pathsForRename(event *sonarrEvent) (map[string]string, error) {
	if event.Series.Path == "" {
		return nil, errors.New("required fields missing")
	}

	paths := make(map[string]string)
	encountered := make(map[string]bool)

	for _, renamedFile := range event.RenamedFiles {
		previousPath := path.Dir(renamedFile.PreviousPath)
		previousFile := path.Base(renamedFile.PreviousPath)
		currentPath := path.Dir(path.Join(event.Series.Path, renamedFile.RelativePath))
		currentFile := path.Base(path.Join(event.Series.Path, renamedFile.RelativePath))

		if _, ok := encountered[previousPath]; !ok {
			encountered[previousPath] = true
			paths[previousPath] = previousFile
		}

		if _, ok := encountered[currentPath]; !ok {
			encountered[currentPath] = true
			paths[currentPath] = currentFile
		}
	}

	return paths, nil
}

var now = time.Now
