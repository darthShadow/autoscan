package plex

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cloudbox/autoscan"
)

func TestScanMatchesLibraryRoot(t *testing.T) {
	for _, test := range []struct {
		name, folder string
		matched      bool
	}{
		{"descendant", "/mnt/media/movies/Movie (2020)", true},
		{"root", "/mnt/media/movies", true},
		{"movies-int", "/mnt/media/movies-int/Movie (2020)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var scans atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/library/sections":
					_, _ = io.WriteString(w, `{"MediaContainer":{"Directory":[{"key":"1","title":"Movies",`+
						`"Location":[{"path":"/mnt/media/movies"}]}]}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/":
					_, _ = io.WriteString(w, `{"MediaContainer":{"version":"1.40.0"}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/library/sections/1/refresh":
					scans.Add(1)
					if got := r.URL.Query().Get("path"); got != test.folder {
						t.Errorf("scan path = %q, want %q", got, test.folder)
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			target, err := New(Config{URL: server.URL, Token: "test-token"})
			if err != nil {
				t.Fatalf("New error = %v, want nil", err)
			}
			err = target.Scan(autoscan.Scan{Folder: test.folder})
			var wantScans int32
			if test.matched {
				wantScans = 1
				if err != nil {
					t.Errorf("Scan error = %v, want nil", err)
				}
			} else if !errors.Is(err, autoscan.ErrLibraryNotMatched) {
				t.Errorf("Scan error = %v, want ErrLibraryNotMatched", err)
			}
			if got := scans.Load(); got != wantScans {
				t.Errorf("scan request count = %d, want %d", got, wantScans)
			}
		})
	}
}
