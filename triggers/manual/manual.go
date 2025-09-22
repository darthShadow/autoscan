package manual

import (
	_ "embed"
	"net/http"
	"path"
	"time"

	"github.com/rs/zerolog/hlog"

	"github.com/cloudbox/autoscan"
)

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

func (h handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rlog := hlog.FromRequest(r)

	query := r.URL.Query()

	switch r.Method {
	case "GET":
		rw.Header().Set("Content-Type", "text/html")
		_, _ = rw.Write(template)
		return
	case "HEAD":
		rw.WriteHeader(http.StatusOK)
		return
	}

	directories := query["dir"]
	paths := query["path"]
	if len(directories) == 0 && len(paths) == 0 {
		rlog.Error().Msg("Manual webhook should receive at least one directory or path")
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	rlog.Trace().Strs("dirs", directories).Strs("paths", paths).Msg("Received directories & paths")

	scans := make([]autoscan.Scan, 0)

	for _, dir := range directories {
		// Rewrite the path based on the provided rewriter.
		folderPath := h.rewrite(path.Clean(dir))

		scans = append(scans, autoscan.Scan{
			Folder:   folderPath,
			Priority: h.priority,
			Time:     now(),
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
			Time:         now(),
		})

	}

	err := h.callback(scans...)
	if err != nil {
		rlog.Error().Err(err).Msg("Processor could not process scans")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusOK)
	for _, scan := range scans {
		rlog.Info().
			Str("path", scan.Folder).
			Str("relative_path", scan.RelativePath).
			Msg("Scan moved to processor")
	}
}

var now = time.Now
