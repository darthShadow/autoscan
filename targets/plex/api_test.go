package plex

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

func TestAPIClientTimeout(t *testing.T) {
	// Server that delays longer than the client timeout
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &apiClient{
		client:  &http.Client{Timeout: 1 * time.Second},
		log:     zerolog.Nop(),
		baseURL: srv.URL,
		token:   "test-token",
	}

	_, err := client.Version()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if !errors.Is(err, autoscan.ErrTargetUnavailable) {
		t.Errorf("expected ErrTargetUnavailable, got: %v", err)
	}
}
