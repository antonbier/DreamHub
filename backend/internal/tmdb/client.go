// Package tmdb is the TMDB connector's client: search for a film or series
// by title, and look up an episode's title — the data behind MediaHub's
// "TMDB" name suggestion (decision D2). An external service, used only on
// an explicit click and only with a key the user configured (PRINCIPLES.md:
// external calls only where a connector explicitly needs them).
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Result is one search hit.
type Result struct {
	Kind  string `json:"kind"` // "movie" or "tv"
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Year  int    `json:"year,omitempty"`
}

// Client is implemented by the real HTTP client and ./mock.
type Client interface {
	// Search finds films and series by title, best matches first.
	Search(ctx context.Context, apiKey, query, language string) ([]Result, error)
	// EpisodeTitle returns a series episode's title ("" if TMDB has none).
	EpisodeTitle(ctx context.Context, apiKey string, tvID int64, season, episode int, language string) (string, error)
}

var (
	ErrNotConfigured = errors.New("tmdb: no API key configured")
	ErrUnauthorized  = errors.New("tmdb: API key rejected")
	errNotFound      = errors.New("tmdb: not found")
)

// HTTPClient calls api.themoviedb.org.
type HTTPClient struct {
	HTTP    *http.Client
	BaseURL string // overridable in tests
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{HTTP: &http.Client{Timeout: 10 * time.Second}, BaseURL: "https://api.themoviedb.org/3"}
}

func (c *HTTPClient) Search(ctx context.Context, apiKey, query, language string) ([]Result, error) {
	var res struct {
		Results []struct {
			MediaType    string `json:"media_type"`
			ID           int64  `json:"id"`
			Title        string `json:"title"`
			Name         string `json:"name"`
			ReleaseDate  string `json:"release_date"`
			FirstAirDate string `json:"first_air_date"`
		} `json:"results"`
	}
	q := url.Values{"query": {query}, "include_adult": {"false"}, "language": {language}}
	if err := c.get(ctx, apiKey, "/search/multi", q, &res); err != nil {
		return nil, err
	}
	var out []Result
	for _, r := range res.Results {
		switch r.MediaType {
		case "movie":
			out = append(out, Result{Kind: "movie", ID: r.ID, Title: r.Title, Year: year(r.ReleaseDate)})
		case "tv":
			out = append(out, Result{Kind: "tv", ID: r.ID, Title: r.Name, Year: year(r.FirstAirDate)})
		}
	}
	return out, nil
}

func (c *HTTPClient) EpisodeTitle(ctx context.Context, apiKey string, tvID int64, season, episode int, language string) (string, error) {
	var res struct {
		Name string `json:"name"`
	}
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", tvID, season, episode)
	err := c.get(ctx, apiKey, path, url.Values{"language": {language}}, &res)
	if errors.Is(err, errNotFound) {
		return "", nil
	}
	return strings.TrimSpace(res.Name), err
}

// get authenticates with either key form TMDB hands out: a v4 "API read
// access token" (a JWT, sent as a bearer token) or a v3 API key (a query
// parameter).
func (c *HTTPClient) get(ctx context.Context, apiKey, path string, q url.Values, v any) error {
	if apiKey == "" {
		return ErrNotConfigured
	}
	bearer := strings.HasPrefix(apiKey, "eyJ")
	if !bearer {
		q.Set("api_key", apiKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: calling %s: %w", path, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return errNotFound
	default:
		return fmt.Errorf("tmdb: %s returned status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func year(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}
