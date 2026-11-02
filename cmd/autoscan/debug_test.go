package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestDebugServerUsesConfigPortAndLoopback(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		port  int
	}{
		{"configured", "port: 3034\n", 13034},
		{"default", "{}\n", 13030},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := decodeConfig(strings.NewReader(test.input))
			if err != nil {
				t.Fatal(err)
			}
			want := net.JoinHostPort("127.0.0.1", strconv.Itoa(test.port))
			if got := debugAddress(cfg.Port); got != want {
				t.Errorf("debug address = %q, want %q", got, want)
			}
			original := listenDebug
			listenDebug = func(ctx context.Context, network, address string) (net.Listener, error) {
				if network != "tcp4" || address != want {
					t.Errorf("listen(%q, %q), want tcp4, %q", network, address, want)
				}
				listener, listenErr := (&net.ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
				if listenErr != nil {
					return nil, fmt.Errorf("listen test debug: %w", listenErr)
				}
				return listener, nil
			}
			t.Cleanup(func() { listenDebug = original })
			server := startDebugServer(cfg.Port)
			if server == nil {
				t.Fatal("debug server did not bind")
			}
			t.Cleanup(func() { _ = server.Close() })
			host, _, splitErr := net.SplitHostPort(server.Addr)
			if splitErr != nil || host != "127.0.0.1" {
				t.Errorf("debug address = %q, want %q", server.Addr, want)
			}
			if server.ReadHeaderTimeout != 5*time.Second {
				t.Errorf("read header timeout = %v, want 5s", server.ReadHeaderTimeout)
			}
			request, err := http.NewRequestWithContext(
				t.Context(), http.MethodGet, "http://"+server.Addr+"/debug/pprof/", http.NoBody,
			)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Timeout: 5 * time.Second}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != http.StatusOK {
				t.Errorf("GET /debug/pprof/ status = %d, want 200", response.StatusCode)
			}
		})
	}
}

func TestDebugOccupiedPortWarnsWithoutStopping(t *testing.T) {
	original, originalLogger := listenDebug, log.Logger
	t.Cleanup(func() { listenDebug, log.Logger = original, originalLogger })
	var output bytes.Buffer
	log.Logger = zerolog.New(&output)
	calls := 0
	listenDebug = func(context.Context, string, string) (net.Listener, error) {
		calls++
		return nil, errors.New("occupied test port")
	}
	if server := startDebugServer(3034); server != nil {
		_ = server.Close()
		t.Fatal("bind failure returned a server")
	}
	if calls != 1 || !strings.Contains(output.String(), `"level":"warn"`) ||
		!strings.Contains(output.String(), "Debug Server Bind Failed") {
		t.Fatalf("listen calls=%d log=%s, want one attempt and Warn", calls, &output)
	}
}
