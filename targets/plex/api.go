// Package plex provides an autoscan target for Plex media servers.
package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/rs/zerolog"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/internal/httpclient"
)

type apiClient struct {
	client  *http.Client
	log     zerolog.Logger
	baseURL string
	token   string
}

func newAPIClient(baseURL, token string, log zerolog.Logger) *apiClient {
	return &apiClient{
		client:  httpclient.New(),
		log:     log,
		baseURL: baseURL,
		token:   token,
	}
}

func (c apiClient) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-Plex-Token", c.token)
	req.Header.Set("Accept", "application/json") // Force JSON Response.

	res, err := c.client.Do(req) //nolint:gosec // URL is user-configured in app config, SSRF is intentional
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, autoscan.ErrTargetUnavailable)
	}

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		res.Body = autoscan.LimitReadCloser(res.Body)
		return res, nil
	}

	c.log.Trace().
		Stringer("request_url", res.Request.URL).
		Int("response_status", res.StatusCode).
		Msg("Request failed")

	// statusCode not in the 2xx range, close response
	_ = res.Body.Close()

	switch res.StatusCode {
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("invalid plex token: %s: %w", res.Status, autoscan.ErrFatal)
	case http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return nil, fmt.Errorf("%s: %w", res.Status, autoscan.ErrTargetUnavailable)
	default:
		return nil, fmt.Errorf("%s: %w", res.Status, autoscan.ErrFatal)
	}
}

func (c apiClient) Version() (string, error) {
	reqURL := autoscan.JoinURL(c.baseURL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed creating version request: %w: %w", err, autoscan.ErrFatal)
	}

	res, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("version: %w", err)
	}

	defer func() { _ = res.Body.Close() }()

	type plexVersionContainer struct {
		Version string `json:"version"`
	}

	type Response struct {
		MediaContainer plexVersionContainer `json:"MediaContainer"`
	}

	resp := new(Response)
	if err := json.NewDecoder(res.Body).Decode(resp); err != nil {
		return "", fmt.Errorf("failed decoding version response: %w: %w", err, autoscan.ErrFatal)
	}

	return resp.MediaContainer.Version, nil
}

type library struct {
	ID   int
	Name string
	Path string
}

func (c apiClient) Libraries() ([]library, error) {
	reqURL := autoscan.JoinURL(c.baseURL, "library", "sections")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed creating libraries request: %w: %w", err, autoscan.ErrFatal)
	}

	res, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("libraries: %w", err)
	}

	defer func() { _ = res.Body.Close() }()

	type plexLocation struct {
		Path string `json:"path"`
	}

	type plexDirectory struct {
		ID       int            `json:"key,string"`
		Name     string         `json:"title"`
		Sections []plexLocation `json:"Location"`
	}

	type plexMediaContainer struct {
		Libraries []plexDirectory `json:"Directory"`
	}

	type Response struct {
		MediaContainer plexMediaContainer `json:"MediaContainer"`
	}

	resp := new(Response)
	if err := json.NewDecoder(res.Body).Decode(resp); err != nil {
		return nil, fmt.Errorf("failed decoding libraries response: %w: %w", err, autoscan.ErrFatal)
	}

	// process response
	libraries := make([]library, 0)
	for _, lib := range resp.MediaContainer.Libraries {
		for _, folder := range lib.Sections {
			libPath := folder.Path

			// Add trailing slash if there is none.
			if libPath != "" && libPath[len(libPath)-1] != '/' {
				libPath += "/"
			}

			libraries = append(libraries, library{
				Name: lib.Name,
				ID:   lib.ID,
				Path: libPath,
			})
		}
	}

	return libraries, nil
}

func (c apiClient) Scan(path string, libraryID int) error {
	reqURL := autoscan.JoinURL(c.baseURL, "library", "sections", strconv.Itoa(libraryID), "refresh")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed creating scan request: %w: %w", err, autoscan.ErrFatal)
	}

	q := url.Values{}
	q.Add("path", path)
	req.URL.RawQuery = q.Encode()

	res, err := c.do(req)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	_ = res.Body.Close()
	return nil
}
