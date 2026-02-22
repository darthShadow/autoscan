package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"

	"github.com/cloudbox/autoscan/processor"
	atrain "github.com/cloudbox/autoscan/triggers/a_train"
	"github.com/cloudbox/autoscan/triggers/lidarr"
	"github.com/cloudbox/autoscan/triggers/manual"
	"github.com/cloudbox/autoscan/triggers/radarr"
	"github.com/cloudbox/autoscan/triggers/readarr"
	"github.com/cloudbox/autoscan/triggers/sonarr"
)

func pattern(name string) string {
	return "/" + name
}

func createCredentials(cfg config) map[string]string {
	creds := make(map[string]string)
	creds[cfg.Auth.Username] = cfg.Auth.Password
	return creds
}

func getRouter(cfg config, proc *processor.Processor) chi.Router {
	mux := chi.NewRouter()

	// Middleware
	mux.Use(middleware.Recoverer)

	// Logging-related middleware
	mux.Use(hlog.NewHandler(log.Logger))
	mux.Use(hlog.RequestIDHandler("id", "request-id"))
	mux.Use(hlog.URLHandler("url"))
	mux.Use(hlog.MethodHandler("method"))
	mux.Use(hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Debug().
			Int("status", status).
			Dur("duration", duration).
			Msg("Request Processed")
	}))

	// Health check
	mux.Get("/health", healthHandler)

	// HTTP-Triggers
	mux.Route("/triggers", func(sub chi.Router) {
		// Use Basic Auth middleware if username and password are set.
		if cfg.Auth.Username != "" && cfg.Auth.Password != "" {
			sub.Use(middleware.BasicAuth("Autoscan 1.x", createCredentials(cfg)))
		}

		// A-Train HTTP-trigger
		sub.Route("/a-train", func(sub chi.Router) {
			trigger, err := atrain.New(cfg.Triggers.ATrain)
			if err != nil {
				log.Fatal().Err(err).Str("trigger", "a-train").Msg("Trigger Init Failed")
			}

			sub.Post("/{drive}", trigger(proc.Add).ServeHTTP)
		})

		// Mixed-style Manual HTTP-trigger
		sub.Route("/manual", func(sub chi.Router) {
			trigger, err := manual.New(cfg.Triggers.Manual)
			if err != nil {
				log.Fatal().Err(err).Str("trigger", "manual").Msg("Trigger Init Failed")
			}

			sub.HandleFunc("/", trigger(proc.Add).ServeHTTP)
		})

		// OLD-style HTTP-triggers. Can be converted to the /{trigger}/{id} format in a 2.0 release.
		for _, t := range cfg.Triggers.Lidarr {
			trigger, err := lidarr.New(t)
			if err != nil {
				log.Fatal().Err(err).Str("trigger", t.Name).Msg("Trigger Init Failed")
			}

			sub.Post(pattern(t.Name), trigger(proc.Add).ServeHTTP)
		}

		for _, t := range cfg.Triggers.Radarr {
			trigger, err := radarr.New(t)
			if err != nil {
				log.Fatal().Err(err).Str("trigger", t.Name).Msg("Trigger Init Failed")
			}

			sub.Post(pattern(t.Name), trigger(proc.Add).ServeHTTP)
		}

		for _, t := range cfg.Triggers.Readarr {
			trigger, err := readarr.New(t)
			if err != nil {
				log.Fatal().Err(err).Str("trigger", t.Name).Msg("Trigger Init Failed")
			}

			sub.Post(pattern(t.Name), trigger(proc.Add).ServeHTTP)
		}

		for _, t := range cfg.Triggers.Sonarr {
			trigger, err := sonarr.New(t)
			if err != nil {
				log.Fatal().Err(err).Str("trigger", t.Name).Msg("Trigger Init Failed")
			}

			sub.Post(pattern(t.Name), trigger(proc.Add).ServeHTTP)
		}
	})

	return mux
}

// Other Handlers
func healthHandler(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	if ready.Load() {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(`{"status":"ready"}`))
	} else {
		rw.WriteHeader(http.StatusServiceUnavailable)
		_, _ = rw.Write([]byte(`{"status":"initializing"}`))
	}
}
