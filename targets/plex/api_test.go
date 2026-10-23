package plex

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
)

func TestAPIClientScanRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	client := newAPIClient(srv.URL, "test-token", zerolog.Nop())
	err := client.Scan("/media/movies", 1)
	if !errors.Is(err, autoscan.ErrScanRejected) {
		t.Errorf("Scan error = %v, want ErrScanRejected", err)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "plex scan: ") {
		t.Errorf("Scan error = %v, want the target kind in its prefix", err)
	}
}

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

func TestAPIClientRejectedRequestLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	var output bytes.Buffer
	client := newAPIClient(srv.URL, "test-token", zerolog.New(&output).Level(zerolog.TraceLevel))
	if _, err := client.Version(); err == nil {
		t.Fatal("Version error = nil, want rejected request")
	}

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	for key, want := range map[string]any{
		"level":           "trace",
		"message":         "Request Failed",
		"request-url":     srv.URL + "/",
		"response-status": float64(http.StatusBadRequest),
	} {
		if got := event[key]; got != want {
			t.Errorf("log[%q] = %v, want %v", key, got, want)
		}
	}
}
