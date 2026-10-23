// Package autoscan provides an autoscan target that forwards scans to another autoscan instance.
package autoscan

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/internal/httpclient"
)

type apiClient struct {
	client  *http.Client
	log     zerolog.Logger
	baseURL string
	user    string
	pass    string
}

func newAPIClient(baseURL, user, pass string, log zerolog.Logger) apiClient {
	return apiClient{
		client:  httpclient.New(),
		log:     log,
		baseURL: baseURL,
		user:    user,
		pass:    pass,
	}
}

func (c apiClient) do(req *http.Request) (*http.Response, error) {
	res, err := c.client.Do(req) //nolint:gosec // URL is user-configured in app config, SSRF is intentional
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, autoscan.ErrTargetUnavailable)
	}

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		res.Body = autoscan.LimitReadCloser(res.Body)
		return res, nil
	}

	c.log.Trace().
		Stringer("request-url", res.Request.URL).
		Int("response-status", res.StatusCode).
		Msg("Request Failed")

	// statusCode not in the 2xx range, close response
	_ = res.Body.Close()

	return nil, fmt.Errorf("request failed: %w",
		autoscan.TargetResponseError(res.StatusCode, res.Status, "invalid basic auth"))
}

func (c apiClient) Available() error {
	// create request
	triggerURL := autoscan.JoinURL(c.baseURL, "triggers", "manual")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, triggerURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed creating head request: %w: %w", err, autoscan.ErrFatal)
	}

	if c.user != "" && c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}

	// send request
	res, err := c.do(req)
	if err != nil {
		return fmt.Errorf("availability: %w", err)
	}

	defer func() { _ = res.Body.Close() }()
	return nil
}

func (c apiClient) Scan(folder, path string) error {
	// create request
	triggerURL := autoscan.JoinURL(c.baseURL, "triggers", "manual")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, triggerURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed creating scan request: %w: %w", err, autoscan.ErrFatal)
	}

	if c.user != "" && c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}

	q := url.Values{}

	if path == "" {
		q.Add("dir", folder)
	} else {
		q.Add("path", path)
	}

	req.URL.RawQuery = q.Encode()

	// send request
	res, err := c.do(req)
	if err != nil {
		return fmt.Errorf("autoscan scan: %w", err)
	}

	defer func() { _ = res.Body.Close() }()
	return nil
}
