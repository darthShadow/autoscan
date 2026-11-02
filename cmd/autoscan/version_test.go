package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterServesVersion(t *testing.T) {
	Version, GitCommit, Timestamp = "1.2.3", "abc1234", "2026-11-02T00:00:00Z"
	t.Cleanup(func() { Version, GitCommit, Timestamp = "", "", "" })
	cfg := config{}
	cfg.Auth.Username, cfg.Auth.Password = "version-reader", "test-password"
	router := getRouter(cfg, nil)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(t.Context(), method, "/version", http.NoBody)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("%s /version status = %d, want 200 without credentials", method, recorder.Code)
			}
			if recorder.Header().Get("Content-Type") != "application/json" {
				t.Errorf("content-type=%q, want application/json", recorder.Header().Get("Content-Type"))
			}
			if method == http.MethodHead {
				return
			}
			var body map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("GET /version body %q: %v", recorder.Body.String(), err)
			}
			want := map[string]string{"version": "1.2.3", "commit": "abc1234", "timestamp": "2026-11-02T00:00:00Z"}
			for key, value := range want {
				if body[key] != value {
					t.Errorf("GET /version %s = %q, want %q", key, body[key], value)
				}
			}
		})
	}
}
