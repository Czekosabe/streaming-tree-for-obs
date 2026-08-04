package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIClient talks to the MediaMTX Control API on loopback.
//
// The wire models below are internal to this package on purpose: they mirror
// MediaMTX's schema, which changes between releases, and must never become the
// public Streaming Tree API contract.
type APIClient struct {
	baseURL string
	client  *http.Client
}

const (
	// apiTimeout is short: the API is on loopback, so a slow answer means the
	// process is wedged rather than busy.
	apiTimeout = 3 * time.Second

	// maxAPIResponseBytes bounds a response body. A path list for one path is
	// a few hundred bytes; 4 MB is generous while still bounded.
	maxAPIResponseBytes = 4 << 20
)

// NewAPIClient builds a client for a loopback base URL such as
// "http://127.0.0.1:9997".
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: apiTimeout},
	}
}

// globalConfig is the subset of /v3/config/global/get this application reads.
//
// MediaMTX v1.19.3 has no dedicated "instance info" endpoint, so the global
// configuration serves as the readiness probe: a valid JSON answer proves the
// API is up and the process finished loading its configuration. The version is
// NOT available here - it is verified from `mediamtx --version` at resolve and
// install time instead.
type globalConfig struct {
	// RTMP and API are required: their presence proves the payload really is a
	// MediaMTX global configuration rather than some other service answering
	// on the same port.
	RTMP *bool `json:"rtmp"`
	API  *bool `json:"api"`
}

// Ping confirms the Control API is answering with a valid payload.
func (c *APIClient) Ping(ctx context.Context) error {
	var config globalConfig
	if err := c.getJSON(ctx, "/v3/config/global/get", &config); err != nil {
		return err
	}

	if config.API == nil || config.RTMP == nil {
		return fmt.Errorf("the Control API answered with an unexpected payload")
	}
	return nil
}

// pathList mirrors GET /v3/paths/list.
//
// Verified against MediaMTX v1.19.3, which answers:
//
//	{"itemCount":1,"pageCount":1,"items":[{"name":"live","ready":false,
//	 "readyTime":null,"source":null,"tracks":[],...}]}
//
// Unknown fields are ignored by encoding/json, so a future release adding
// fields does not break this client.
type pathList struct {
	ItemCount int        `json:"itemCount"`
	Items     []pathItem `json:"items"`
}

type pathItem struct {
	// Name is required: without it an item cannot be matched to the configured
	// ingest path, so a missing name is a malformed response.
	Name *string `json:"name"`
	// Ready is required: it is the signal the whole ingest state depends on.
	Ready     *bool       `json:"ready"`
	ReadyTime *string     `json:"readyTime"`
	Source    *pathSource `json:"source"`
	Tracks    []string    `json:"tracks"`
}

type pathSource struct {
	Type string `json:"type"`
}

// PathStatus is this package's own view of one MediaMTX path.
type PathStatus struct {
	// Found is false when the configured path is absent from the list.
	Found bool
	// Ready is true when a publisher is connected and the path is usable.
	Ready bool
	// SourceType is the MediaMTX source kind, e.g. "rtmpConn". Empty when no
	// publisher is connected.
	SourceType string
	// ReadyTime is when the path became ready, as reported by MediaMTX.
	ReadyTime string
	// Tracks are the codec identifiers MediaMTX detected, e.g. ["H264"].
	Tracks []string
}

// PathStatusFor fetches the list and returns the status of one path.
func (c *APIClient) PathStatusFor(ctx context.Context, name string) (PathStatus, error) {
	var list pathList
	if err := c.getJSON(ctx, "/v3/paths/list", &list); err != nil {
		return PathStatus{}, err
	}

	for index, item := range list.Items {
		// Required fields are validated strictly: silently treating a missing
		// "ready" as false would report "waiting" during an API problem.
		if item.Name == nil {
			return PathStatus{}, fmt.Errorf("path item %d has no name", index)
		}
		if item.Ready == nil {
			return PathStatus{}, fmt.Errorf("path %q has no ready flag", *item.Name)
		}

		if *item.Name != name {
			continue
		}

		status := PathStatus{
			Found:  true,
			Ready:  *item.Ready,
			Tracks: item.Tracks,
		}
		if item.Source != nil {
			status.SourceType = item.Source.Type
		}
		if item.ReadyTime != nil {
			status.ReadyTime = *item.ReadyTime
		}
		if status.Tracks == nil {
			status.Tracks = []string{}
		}
		return status, nil
	}

	return PathStatus{Found: false, Tracks: []string{}}, nil
}

// getJSON performs one bounded GET and decodes the body.
func (c *APIClient) getJSON(ctx context.Context, path string, target any) error {
	requestCtx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build the Control API request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("call the Control API: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the Control API answered %d for %s", response.StatusCode, path)
	}

	// Read one byte past the budget so an oversized body is detectable.
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAPIResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read the Control API response: %w", err)
	}
	if len(body) > maxAPIResponseBytes {
		return fmt.Errorf("the Control API response exceeds %d bytes", maxAPIResponseBytes)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("parse the Control API response: %w", err)
	}
	return nil
}
