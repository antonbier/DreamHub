// Package tvmaze is the TVmaze connector's client: a series' complete
// episode list (season, number, title, first air date) — what SeriesHub
// needs to say which episodes are missing, and what neither the box nor
// SerienRecorder's confirmed endpoints provide. TVmaze's public API needs
// no key; it's still an external service, used only while the connector
// is switched on (Settings → TVmaze), and its answers are cached by the
// caller (SeriesHub, internal/hubs/series_episodes.go).
package tvmaze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Show is one search hit.
type Show struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Premiered string  `json:"premiered,omitempty"` // YYYY-MM-DD
	Language  string  `json:"language,omitempty"`
	Score     float64 `json:"score,omitempty"`
}

// Episode is one episode of a show. Specials (no number) are left out.
type Episode struct {
	Season   int    `json:"season"`
	Number   int    `json:"number"`
	Name     string `json:"name"`
	Airdate  string `json:"airdate,omitempty"`  // YYYY-MM-DD, the first broadcast in the show's home country
	Airstamp int64  `json:"airstamp,omitempty"` // Unix seconds, UTC; 0 when unknown
}

// Client is implemented by HTTPClient and ./mock.
type Client interface {
	SearchShows(ctx context.Context, name string) ([]Show, error)
	Episodes(ctx context.Context, showID int64) ([]Episode, error)
}

// ErrRateLimited is TVmaze's 429 (20 calls per 10 s per IP).
var ErrRateLimited = errors.New("tvmaze: rate limited")

// HTTPClient calls TVmaze's public API.
type HTTPClient struct {
	HTTP    *http.Client
	BaseURL string // overridable in tests
}

// NewHTTPClient returns a client for the real API.
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{HTTP: &http.Client{Timeout: 10 * time.Second}, BaseURL: "https://api.tvmaze.com"}
}

func (c *HTTPClient) SearchShows(ctx context.Context, name string) ([]Show, error) {
	var res []struct {
		Score float64 `json:"score"`
		Show  struct {
			ID        int64  `json:"id"`
			Name      string `json:"name"`
			Premiered string `json:"premiered"`
			Language  string `json:"language"`
		} `json:"show"`
	}
	if err := c.get(ctx, "/search/shows?"+url.Values{"q": {name}}.Encode(), &res); err != nil {
		return nil, err
	}
	out := make([]Show, 0, len(res))
	for _, r := range res {
		out = append(out, Show{ID: r.Show.ID, Name: r.Show.Name, Premiered: r.Show.Premiered, Language: r.Show.Language, Score: r.Score})
	}
	return out, nil
}

func (c *HTTPClient) Episodes(ctx context.Context, showID int64) ([]Episode, error) {
	var res []struct {
		Season   int     `json:"season"`
		Number   *int    `json:"number"`
		Name     string  `json:"name"`
		Airdate  string  `json:"airdate"`
		Airstamp *string `json:"airstamp"`
	}
	if err := c.get(ctx, "/shows/"+strconv.FormatInt(showID, 10)+"/episodes", &res); err != nil {
		return nil, err
	}
	out := make([]Episode, 0, len(res))
	for _, r := range res {
		if r.Number == nil || r.Season <= 0 {
			continue // a special
		}
		e := Episode{Season: r.Season, Number: *r.Number, Name: r.Name, Airdate: r.Airdate}
		if r.Airstamp != nil {
			if t, err := time.Parse(time.RFC3339, *r.Airstamp); err == nil {
				e.Airstamp = t.Unix()
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// maxBody guards against an unexpectedly huge answer (a long-running soap
// has a few thousand episodes, well under this).
const maxBody = 16 << 20

func (c *HTTPClient) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("tvmaze: calling %s: %w", path, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("tvmaze: %s returned status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(v)
}
