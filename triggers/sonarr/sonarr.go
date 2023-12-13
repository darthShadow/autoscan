package sonarr

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/rs/zerolog/hlog"

	"github.com/cloudbox/autoscan"
)

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
		return nil, err
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

type sonarrEvent struct {
	Type string `json:"eventType"`

	File struct {
		RelativePath string
	} `json:"episodeFile"`

	Series struct {
		Path string
	} `json:"series"`

	RenamedFiles []struct {
		// use PreviousPath as the Series.Path might have changed.
		PreviousPath string
		RelativePath string
	} `json:"renamedEpisodeFiles"`
}

func (h handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var err error
	rlog := hlog.FromRequest(r)

	event := new(sonarrEvent)
	err = json.NewDecoder(r.Body).Decode(event)
	if err != nil {
		rlog.Error().Err(err).Msg("Failed decoding request")
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	rlog.Trace().Interface("event", event).Msg("Received JSON body")

	if strings.EqualFold(event.Type, "Test") {
		rlog.Info().Msg("Received test event")
		rw.WriteHeader(http.StatusOK)
		return
	}

	paths := make(map[string]string)

	// a Download event is either an upgrade or a new file.
	// the EpisodeFileDelete event shares the same request format as Download.
	if strings.EqualFold(event.Type, "Download") || strings.EqualFold(event.Type, "EpisodeFileDelete") {
		if event.File.RelativePath == "" || event.Series.Path == "" {
			rlog.Error().Msg("Required fields are missing")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		// Use path.Dir to get the directory in which the file is located
		folderPath := path.Dir(path.Join(event.Series.Path, event.File.RelativePath))
		// Use path.Base to get the filename
		filePath := path.Base(path.Join(event.Series.Path, event.File.RelativePath))
		paths[folderPath] = filePath
	}

	// An entire show has been deleted
	if strings.EqualFold(event.Type, "SeriesDelete") {
		if event.Series.Path == "" {
			rlog.Error().Msg("Required fields are missing")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		// Scan the folder of the show
		paths[event.Series.Path] = ""
	}

	if strings.EqualFold(event.Type, "Rename") {
		if event.Series.Path == "" {
			rlog.Error().Msg("Required fields are missing")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		// Keep track of which paths we have already added to paths.
		encountered := make(map[string]bool)

		for _, renamedFile := range event.RenamedFiles {
			previousPath := path.Dir(renamedFile.PreviousPath)
			previousFile := path.Base(renamedFile.PreviousPath)
			currentPath := path.Dir(path.Join(event.Series.Path, renamedFile.RelativePath))
			currentFile := path.Base(path.Join(event.Series.Path, renamedFile.RelativePath))

			// if previousPath not in paths, then add it.
			if _, ok := encountered[previousPath]; !ok {
				encountered[previousPath] = true
				paths[previousPath] = previousFile
			}

			// if currentPath not in paths, then add it.
			if _, ok := encountered[currentPath]; !ok {
				encountered[currentPath] = true
				paths[currentPath] = currentFile
			}
		}
	}

	var scans []autoscan.Scan

	for folderPath, filePath := range paths {
		folderPath := h.rewrite(folderPath)

		scan := autoscan.Scan{
			Folder:       folderPath,
			RelativePath: filePath,
			Priority:     h.priority,
			Time:         now(),
		}

		scans = append(scans, scan)
	}

	err = h.callback(scans...)
	if err != nil {
		rlog.Error().Err(err).Msg("Processor could not process scans")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, scan := range scans {
		rlog.Info().
			Str("path", scan.Folder).
			Str("event", event.Type).
			Msg("Scan moved to processor")
	}

	rw.WriteHeader(http.StatusOK)
}

var now = time.Now
