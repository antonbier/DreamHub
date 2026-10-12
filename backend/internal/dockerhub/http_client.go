package dockerhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// callTimeout bounds one call to Docker Hub — the same not-forever-hung
// choice every other outbound connector in this repo makes.
const callTimeout = 8 * time.Second

// manifestTTL is how long a Manifest result is trusted before being
// fetched again — long enough that browsing a few search results in one
// sitting doesn't repeat lookups, short enough that a maintainer adding
// arm64 support later isn't hidden behind a permanently stale "no" (see
// the package doc).
const manifestTTL = 10 * time.Minute

// HTTPClient calls Docker Hub's own v2 API. BaseURL defaults to the real
// one; tests override it to a local httptest.Server.
type HTTPClient struct {
	BaseURL string
	// Transport overrides http.DefaultTransport (tests).
	Transport http.RoundTripper

	mu    sync.Mutex
	cache map[string]manifestCacheEntry // "image@tag" -> entry
}

type manifestCacheEntry struct {
	arches []Arch
	notErr bool // whether this entry caches an ErrNotFound result
	at     time.Time
}

var _ Client = &HTTPClient{}

// NewHTTPClient returns a client against the real Docker Hub.
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{BaseURL: "https://hub.docker.com"}
}

func (c *HTTPClient) client() *http.Client {
	return &http.Client{Timeout: callTimeout, Transport: c.Transport}
}

func (c *HTTPClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return resp, nil
}

type searchResponse struct {
	Results []struct {
		RepoName    string `json:"repo_name"`
		Description string `json:"short_description"`
		Stars       int    `json:"star_count"`
	} `json:"results"`
}

func (c *HTTPClient) Search(ctx context.Context, query string) ([]Repo, error) {
	q := url.Values{"query": {query}, "page_size": {"25"}}
	resp, err := c.get(ctx, "/v2/search/repositories/?"+q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: search: status %d", ErrUnavailable, resp.StatusCode)
	}
	var parsed searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([]Repo, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		out = append(out, Repo{Name: r.RepoName, Description: r.Description, Stars: r.Stars})
	}
	return out, nil
}

type tagResponse struct {
	Images []struct {
		Architecture string `json:"architecture"`
		Variant      string `json:"variant"`
	} `json:"images"`
}

func (c *HTTPClient) Manifest(ctx context.Context, image, tag string) ([]Arch, error) {
	key := image + "@" + tag
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]manifestCacheEntry{}
	}
	if e, ok := c.cache[key]; ok && time.Since(e.at) < manifestTTL {
		c.mu.Unlock()
		if e.notErr {
			return nil, ErrNotFound
		}
		return e.arches, nil
	}
	c.mu.Unlock()

	arches, err := c.fetchManifest(ctx, image, tag)

	c.mu.Lock()
	switch {
	case err == nil:
		c.cache[key] = manifestCacheEntry{arches: arches, at: time.Now()}
	case err == ErrNotFound:
		c.cache[key] = manifestCacheEntry{notErr: true, at: time.Now()}
	}
	c.mu.Unlock()

	return arches, err
}

// fetchManifest builds the request path without escaping image: it is a
// Docker Hub "namespace/repo" pair (validated by the caller,
// hubs.validDockerImage, before this is ever reached), and url.PathEscape
// would wrongly encode its "/" as "%2F".
func (c *HTTPClient) fetchManifest(ctx context.Context, image, tag string) ([]Arch, error) {
	path := "/v2/repositories/" + image + "/tags/" + url.PathEscape(tag)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: manifest: status %d", ErrUnavailable, resp.StatusCode)
	}
	var parsed tagResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([]Arch, 0, len(parsed.Images))
	for _, img := range parsed.Images {
		out = append(out, Arch{Architecture: img.Architecture, Variant: img.Variant})
	}
	return out, nil
}
