package silo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cloudbox/autoscan"
	"github.com/cloudbox/autoscan/internal/httpclient"
)

type library struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Paths   []string `json:"paths"`
	Enabled bool     `json:"enabled"`
}

type libraryResponse struct {
	Items []library `json:"items"`
}

type scanRequest struct {
	Path string `json:"path"`
}

type scanResponse struct {
	Mode      string `json:"mode"`
	LibraryID string `json:"library_id"`
}

type scanResult struct {
	response  scanResponse
	decodeErr error
}

type problem struct {
	Detail string `json:"detail"`
}

type apiClient struct {
	client  *http.Client
	baseURL string
	token   string
}

func newAPIClient(baseURL, token string) apiClient {
	return apiClient{
		client: httpclient.New(), baseURL: baseURL, token: token,
	}
}

func (c apiClient) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.client.Do(req) //nolint:gosec // URL is user-configured in app config, SSRF is intentional
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, autoscan.ErrTargetUnavailable)
	}
	res.Body = autoscan.LimitReadCloser(res.Body)
	if res.StatusCode >= http.StatusOK && res.StatusCode < http.StatusMultipleChoices {
		return res, nil
	}
	defer func() { _ = res.Body.Close() }()

	var response problem
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		response.Detail = ""
	}
	shared := autoscan.TargetResponseError(res.StatusCode, res.Status, "invalid silo token")
	if response.Detail != "" {
		return nil, fmt.Errorf("request failed: %s: %w", response.Detail, shared)
	}
	return nil, fmt.Errorf("request failed: %w", shared)
}

func (c apiClient) libraries() ([]library, error) {
	reqURL := autoscan.JoinURL(c.baseURL, "api", "v2", "libraries")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed creating libraries request: %w: %w", err, autoscan.ErrFatal)
	}
	res, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("libraries: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	var response libraryResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed decoding libraries response: %w: %w", err, autoscan.ErrFatal)
	}
	return response.Items, nil
}

func (c apiClient) available() error {
	reqURL := autoscan.JoinURL(c.baseURL, "api", "v2", "scan", "capabilities")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("silo availability: failed creating request: %w: %w", err, autoscan.ErrFatal)
	}
	res, err := c.do(req)
	if err != nil {
		return fmt.Errorf("silo availability: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	return nil
}

// scan keeps decode errors in the result because Silo has already accepted the request.
func (c apiClient) scan(folder string) (scanResult, error) {
	payload, err := json.Marshal(scanRequest{Path: folder}) //nolint:errchkjson // Only a string field; cannot fail.
	if err != nil {
		return scanResult{}, fmt.Errorf("failed encoding scan request: %w: %w", err, autoscan.ErrFatal)
	}
	reqURL := autoscan.JoinURL(c.baseURL, "api", "v2", "scan")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return scanResult{}, fmt.Errorf("failed creating scan request: %w: %w", err, autoscan.ErrFatal)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.do(req)
	if err != nil {
		return scanResult{}, err
	}
	defer func() { _ = res.Body.Close() }()

	var result scanResult
	if err := json.NewDecoder(res.Body).Decode(&result.response); err != nil {
		result.decodeErr = fmt.Errorf("failed decoding scan response: %w", err)
	}
	return result, nil
}
