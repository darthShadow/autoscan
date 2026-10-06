package autoscan

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/cloudbox/autoscan"
)

func TestScan(t *testing.T) {
	type Test struct {
		Name      string
		Rewrite   []autoscan.Rewrite
		Scan      autoscan.Scan
		WantQuery url.Values
	}

	testCases := []Test{
		{
			Name: "rewrites file path",
			Rewrite: []autoscan.Rewrite{
				{From: "/mnt/unionfs/", To: "/mnt/media/"},
			},
			Scan: autoscan.Scan{
				Folder: "/mnt/unionfs/movies/X", RelativePath: "pollermovie.mkv",
			},
			WantQuery: url.Values{"path": {"/mnt/media/movies/X/pollermovie.mkv"}},
		},
		{
			Name: "keeps file path without rewrite",
			Scan: autoscan.Scan{
				Folder: "/mnt/unionfs/movies/X", RelativePath: "pollermovie.mkv",
			},
			WantQuery: url.Values{"path": {"/mnt/unionfs/movies/X/pollermovie.mkv"}},
		},
		{
			Name: "rewrites folder without file",
			Rewrite: []autoscan.Rewrite{
				{From: "/mnt/unionfs/", To: "/mnt/media/"},
			},
			Scan:      autoscan.Scan{Folder: "/mnt/unionfs/movies/X"},
			WantQuery: url.Values{"dir": {"/mnt/media/movies/X"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			type request struct {
				method string
				path   string
				query  url.Values
			}

			var requests []request
			var mu sync.Mutex
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{method: r.Method, path: r.URL.Path, query: r.URL.Query()})
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(srv.Close)

			tgt, err := New(Config{URL: srv.URL, Rewrite: tc.Rewrite})
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}

			if err := tgt.Scan(tc.Scan); err != nil {
				t.Fatalf("Scan() error = %v, want nil", err)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(requests) != 1 {
				t.Fatalf("request count = %d, want 1", len(requests))
			}

			got := requests[0]
			if got.method != http.MethodPost {
				t.Errorf("method = %q, want %q", got.method, http.MethodPost)
			}
			if got.path != "/triggers/manual" {
				t.Errorf("URL path = %q, want %q", got.path, "/triggers/manual")
			}
			if got.query.Encode() != tc.WantQuery.Encode() {
				t.Errorf("query = %v, want %v", got.query, tc.WantQuery)
			}
		})
	}
}
