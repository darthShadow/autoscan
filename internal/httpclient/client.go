package httpclient

import (
	"net/http"
	"time"
)

const targetTimeout = 30 * time.Second

// New returns an HTTP client with a sensible timeout for target API calls.
func New() *http.Client {
	return &http.Client{Timeout: targetTimeout}
}
