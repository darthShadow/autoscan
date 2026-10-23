package silo

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/cloudbox/autoscan"
)

const libraryFixture = `{"items":[
	{"id":"7","name":"Movies 4K","paths":["/mnt/media/movies/4k-en"],"enabled":true},
	{"id":"8","name":"Movies HD","paths":["/mnt/media/movies/hd-en"],"enabled":false},
	{"id":"9","name":"TV","paths":["/mnt/media/tv/",""],"enabled":true}
]}`

const acceptedBody = `{"status":"accepted","mode":"subtree","library_id":"7"}`

type recordedRequest struct {
	method      string
	path        string
	contentType string
	body        string
}

type fakeSilo struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

func newFakeSilo(t *testing.T, scanStatus int, scanType, scanBody string, capabilitiesStatus int) *fakeSilo {
	t.Helper()
	fake := &fakeSilo{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		fake.mu.Lock()
		fake.requests = append(fake.requests, recordedRequest{
			method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"), body: string(body),
		})
		fake.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/libraries":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, libraryFixture)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/scan/capabilities":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(capabilitiesStatus)
			if capabilitiesStatus == http.StatusOK {
				_, _ = io.WriteString(w, `{"state":"available"}`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/scan":
			if scanType != "" {
				w.Header().Set("Content-Type", scanType)
			}
			w.WriteHeader(scanStatus)
			_, _ = io.WriteString(w, scanBody)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeSilo) requestsFor(method, requestPath string) []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var matches []recordedRequest
	for _, request := range f.requests {
		if request.method == method && request.path == requestPath {
			matches = append(matches, request)
		}
	}
	return matches
}

func newTestTarget(t *testing.T, fake *fakeSilo) autoscan.Target {
	t.Helper()
	target, err := New(Config{URL: fake.server.URL, Token: "test-token"})
	if err != nil {
		t.Fatalf("New error = %v, want nil", err)
	}
	return target
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&output)
	t.Cleanup(func() { log.Logger = previous })
	return &output
}

func logEvents(t *testing.T, output *bytes.Buffer, message string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for line := range bytes.SplitSeq(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("decode log event %q: %v", line, err)
		}
		if event["message"] == message {
			events = append(events, event)
		}
	}
	return events
}

func TestNewReadsEnabledLibraries(t *testing.T) {
	fake := newFakeSilo(t, http.StatusAccepted, "application/json", acceptedBody, http.StatusOK)
	target := newTestTarget(t, fake)
	err := target.Scan(autoscan.Scan{Folder: "/mnt/media/movies/hd-en/Movie (2020)"})
	if !errors.Is(err, autoscan.ErrLibraryNotMatched) {
		t.Errorf("Scan error = %v, want ErrLibraryNotMatched", err)
	}
	if got := len(fake.requestsFor(http.MethodPost, "/api/v2/scan")); got != 0 {
		t.Errorf("scan POST count = %d, want 0", got)
	}
	if err := target.Available(); err != nil {
		t.Errorf("Available error = %v, want nil", err)
	}
	if got := len(fake.requestsFor(http.MethodGet, "/api/v2/libraries")); got != 1 {
		t.Errorf("libraries GET count = %d after Scan and Available, want 1", got)
	}
}

func TestScanLocalMatch(t *testing.T) {
	for _, test := range []struct {
		name    string
		folder  string
		matched bool
	}{
		{"descendant", "/mnt/media/movies/4k-en/Movie (2020)", true},
		{"root", "/mnt/media/movies/4k-en", true},
		{"cleaned", "/mnt/media/movies/4k-en/./Movie (2020)//", true},
		{"sibling", "/mnt/media/movies/4k-en-x/Movie", false},
		{"unionfs", "/mnt/unionfs/movies/4k-en/Movie (2020)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeSilo(t, http.StatusAccepted, "application/json", acceptedBody, http.StatusOK)
			target := newTestTarget(t, fake)
			err := target.Scan(autoscan.Scan{Folder: test.folder})
			posts := fake.requestsFor(http.MethodPost, "/api/v2/scan")
			if got := len(fake.requestsFor(http.MethodGet, "/api/v2/libraries")); got != 1 {
				t.Errorf("libraries GET count = %d after one New and Scan, want 1", got)
			}
			if !test.matched {
				if !errors.Is(err, autoscan.ErrLibraryNotMatched) {
					t.Errorf("Scan error = %v, want ErrLibraryNotMatched", err)
				}
				if len(posts) != 0 {
					t.Errorf("scan POST count = %d, want 0", len(posts))
				}
				return
			}
			if err != nil {
				t.Fatalf("Scan error = %v, want nil", err)
			}
			if len(posts) != 1 {
				t.Fatalf("scan POST count = %d, want 1", len(posts))
			}
			var body struct {
				Path string `json:"path"`
			}
			if decodeErr := json.Unmarshal([]byte(posts[0].body), &body); decodeErr != nil {
				t.Fatalf("decode scan body: %v", decodeErr)
			}
			if want := path.Clean(test.folder); body.Path != want {
				t.Errorf("POST path = %q, want %q", body.Path, want)
			}
		})
	}
}

func TestScanSendsFolderOnly(t *testing.T) {
	fake := newFakeSilo(t, http.StatusAccepted, "application/json", acceptedBody, http.StatusOK)
	target := newTestTarget(t, fake)
	folder := "/mnt/media/tv/x/Season 01"
	if err := target.Scan(autoscan.Scan{Folder: folder, RelativePath: "pollershow.mkv"}); err != nil {
		t.Fatalf("Scan error = %v, want nil", err)
	}
	posts := fake.requestsFor(http.MethodPost, "/api/v2/scan")
	if len(posts) != 1 {
		t.Fatalf("scan POST count = %d, want 1", len(posts))
	}
	if got := posts[0].contentType; got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(posts[0].body), &body); err != nil {
		t.Fatalf("decode scan body: %v", err)
	}
	if len(body) != 1 || body["path"] != folder {
		t.Errorf("POST body = %#v, want exactly map[path:%s]", body, folder)
	}
}

func TestScanAccepted(t *testing.T) {
	output := captureLogs(t)
	fake := newFakeSilo(t, http.StatusAccepted, "application/json", acceptedBody, http.StatusOK)
	target := newTestTarget(t, fake)
	folder := "/mnt/media/movies/4k-en/Movie (2020)"
	if err := target.Scan(autoscan.Scan{Folder: folder}); err != nil {
		t.Fatalf("Scan error = %v, want nil", err)
	}
	events := logEvents(t, output, "Scan Accepted")
	if len(events) != 1 {
		t.Fatalf("Scan Accepted event count = %d, want 1", len(events))
	}
	for key, want := range map[string]string{
		"level": "info", "path": folder, "library": "Movies 4K", "library-id": "7", "mode": "subtree",
	} {
		if got := events[0][key]; got != want {
			t.Errorf("log[%q] = %v, want %q", key, got, want)
		}
	}
}

func TestScanClassifiesResponse(t *testing.T) {
	for _, test := range []struct {
		name        string
		status      int
		contentType string
		body        string
		wantError   error
		wantDetail  string
	}{
		{
			name: "missing path", status: http.StatusBadRequest, contentType: "application/problem+json",
			body: `{"type":"https://siloserver.org/docs/api/v2/problems/path_missing","title":"Bad Request",` +
				`"status":400,"detail":"Path does not exist","instance":"urn:silo:request:1"}`,
			wantError: autoscan.ErrScanRejected, wantDetail: "Path does not exist",
		},
		{
			name: "unauthorized", status: http.StatusUnauthorized,
			wantError: autoscan.ErrFatal, wantDetail: "invalid silo token",
		},
		{
			name: "unavailable", status: http.StatusServiceUnavailable, contentType: "application/problem+json",
			body:       `{"status":503,"detail":"The scanner service is not available on this server."}`,
			wantError:  autoscan.ErrTargetUnavailable,
			wantDetail: "The scanner service is not available on this server.",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeSilo(t, test.status, test.contentType, test.body, http.StatusOK)
			target := newTestTarget(t, fake)
			folder := "/mnt/media/movies/4k-en/Movie (2020)"
			err := target.Scan(autoscan.Scan{Folder: folder})
			if !errors.Is(err, test.wantError) {
				t.Errorf("Scan error = %v, want %v", err, test.wantError)
			}
			if err == nil {
				t.Fatal("Scan error = nil, want classified response error")
			}
			for _, part := range []string{"silo", folder, test.wantDetail} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Scan error = %q, want it to contain %q", err, part)
				}
			}
		})
	}
}

func TestScanUnreadableAcceptedBody(t *testing.T) {
	output := captureLogs(t)
	fake := newFakeSilo(t, http.StatusAccepted, "application/json", "not json", http.StatusOK)
	target := newTestTarget(t, fake)
	folder := "/mnt/media/movies/4k-en/Movie (2020)"
	if err := target.Scan(autoscan.Scan{Folder: folder}); err != nil {
		t.Fatalf("Scan error = %v, want nil", err)
	}
	events := logEvents(t, output, "Scan Response Unreadable")
	if len(events) != 1 {
		t.Fatalf("Scan Response Unreadable event count = %d, want 1", len(events))
	}
	for key, want := range map[string]string{"level": "warn", "path": folder} {
		if got := events[0][key]; got != want {
			t.Errorf("log[%q] = %v, want %q", key, got, want)
		}
	}
	if message, ok := events[0]["error"].(string); !ok || message == "" {
		t.Errorf("log[error] = %v, want non-empty string", events[0]["error"])
	}
	if got := len(logEvents(t, output, "Scan Accepted")); got != 0 {
		t.Errorf("Scan Accepted event count = %d, want 0", got)
	}
}

func TestAvailable(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
	}{
		{"available", http.StatusOK},
		{"unauthorized", http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeSilo(t, http.StatusAccepted, "application/json", acceptedBody, test.status)
			target := newTestTarget(t, fake)
			err := target.Available()
			if got := len(fake.requestsFor(http.MethodGet, "/api/v2/scan/capabilities")); got != 1 {
				t.Errorf("capabilities GET count = %d, want 1", got)
			}
			if test.status == http.StatusOK {
				if err != nil {
					t.Errorf("Available error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, autoscan.ErrFatal) {
				t.Errorf("Available error = %v, want ErrFatal", err)
			}
			if err == nil || !strings.Contains(err.Error(), "silo availability") {
				t.Errorf("Available error = %v, want silo availability prefix", err)
			}
		})
	}
}
