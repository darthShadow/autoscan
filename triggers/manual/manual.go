// Package manual provides an autoscan trigger for manual HTTP scan requests.
package manual

import (
	_ "embed"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/rs/zerolog/hlog"

	"github.com/cloudbox/autoscan"
)

// Config holds configuration for the manual trigger.
type Config struct {
	Rewrite   []autoscan.Rewrite `yaml:"rewrite"`
	Priority  int                `yaml:"priority"`
	Verbosity string             `yaml:"verbosity"`
}

//go:embed "template.html"
var template []byte

// New creates an autoscan-compatible HTTP Trigger for manual webhooks.
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

func (h handler) ServeHTTP(writer http.ResponseWriter, r *http.Request) {
	rlog := hlog.FromRequest(r)

	query := r.URL.Query()

	switch r.Method {
	case http.MethodGet:
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write(template)
		return
	case http.MethodHead:
		writer.WriteHeader(http.StatusOK)
		return
	}

	directories := query["dir"]
	paths := query["path"]
	if len(directories) == 0 && len(paths) == 0 {
		rlog.Error().Msg("Empty Request")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	rlog.Trace().Strs("dirs", directories).Strs("paths", paths).Msg("Webhook Payload")

	scans := make([]autoscan.Scan, 0)

	for _, dir := range directories {
		// Rewrite the path based on the provided rewriter.
		folderPath := h.rewrite(path.Clean(dir))

		scans = append(scans, autoscan.Scan{
			Folder:   folderPath,
			Priority: h.priority,
			Time:     now().Unix(),
		})
	}

	for _, p := range paths {
		folder, relativePath := path.Split(path.Clean(p))

		// Rewrite the path based on the provided rewriter.
		folderPath := h.rewrite(path.Clean(folder))

		scans = append(scans, autoscan.Scan{
			Folder:       folderPath,
			RelativePath: relativePath,
			Priority:     h.priority,
			Time:         now().Unix(),
		})
	}

	err := h.callback(scans...)
	if err != nil {
		rlog.Error().Err(err).Msg("Scan Enqueue Failed")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.WriteHeader(http.StatusOK)
	for _, scan := range scans {
		rlog.Info().
			Str("path", scan.Folder).
			Str("relative_path", scan.RelativePath).
			Msg("Scan Enqueued")
	}
}

var now = time.Now
