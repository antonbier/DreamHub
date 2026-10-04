// Package omdb is the OMDb connector's client: the ratings TMDB doesn't
// have — Rotten Tomatoes' Tomatometer, IMDb's rating and Metacritic's
// Metascore — for one title by its IMDb ID (which TMDB supplies, so
// nothing is guessed from a title search). OMDb needs a free API key
// (1,000 requests a day); its answers are cached by the caller
// (internal/enrich), so a title is asked about at most once a week.
package omdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Ratings is what OMDb knows; zero values mean "no rating there".
type Ratings struct {
	IMDb           float64 `json:"imdb,omitempty"`      // 0-10
	IMDbVotes      int     `json:"imdbVotes,omitempty"` // number of votes
	RottenTomatoes int     `json:"rottenTomatoes,omitempty"`
	Metascore      int     `json:"metascore,omitempty"` // 0-100
}

// Client is implemented by HTTPClient and ./mock.
type Client interface {
	// Ratings looks one title up by its IMDb ID ("tt0133093"). A title
	// OMDb doesn't know is zero Ratings, not an error.
	Ratings(ctx context.Context, apiKey, imdbID string) (Ratings, error)
}

var (
	ErrNotConfigured = errors.New("omdb: no API key configured")
	ErrUnauthorized  = errors.New("omdb: API key rejected or daily limit reached")
	ErrInvalidID     = errors.New("omdb: not an IMDb ID")
)

// ValidIMDbID accepts only IMDb's own title IDs.
var validID = regexp.MustCompile(`^tt\d{5,10}$`)

func ValidIMDbID(id string) bool { return validID.MatchString(id) }

// HTTPClient calls www.omdbapi.com.
type HTTPClient struct {
	HTTP    *http.Client
	BaseURL string // overridable in tests
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{HTTP: &http.Client{Timeout: 10 * time.Second}, BaseURL: "https://www.omdbapi.com/"}
}

func (c *HTTPClient) Ratings(ctx context.Context, apiKey, imdbID string) (Ratings, error) {
	if apiKey == "" {
		return Ratings{}, ErrNotConfigured
	}
	if !ValidIMDbID(imdbID) {
		return Ratings{}, ErrInvalidID
	}
	q := url.Values{"apikey": {apiKey}, "i": {imdbID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"?"+q.Encode(), nil)
	if err != nil {
		return Ratings{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// Never the URL: it carries the key.
		return Ratings{}, errors.New("omdb: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return Ratings{}, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return Ratings{}, fmt.Errorf("omdb: status %d", resp.StatusCode)
	}
	var d struct {
		Response   string `json:"Response"`
		Error      string `json:"Error"`
		IMDbRating string `json:"imdbRating"`
		IMDbVotes  string `json:"imdbVotes"`
		Metascore  string `json:"Metascore"`
		Ratings    []struct {
			Source string `json:"Source"`
			Value  string `json:"Value"`
		} `json:"Ratings"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return Ratings{}, err
	}
	if d.Response == "False" {
		if strings.Contains(strings.ToLower(d.Error), "api key") || strings.Contains(strings.ToLower(d.Error), "limit") {
			return Ratings{}, ErrUnauthorized
		}
		return Ratings{}, nil // not found
	}
	return parse(d.IMDbRating, d.IMDbVotes, d.Metascore, func(yield func(src, val string)) {
		for _, r := range d.Ratings {
			yield(r.Source, r.Value)
		}
	}), nil
}

func parse(imdb, votes, meta string, ratings func(func(src, val string))) Ratings {
	var r Ratings
	r.IMDb, _ = strconv.ParseFloat(imdb, 64) // "N/A" → 0
	r.IMDbVotes, _ = strconv.Atoi(strings.ReplaceAll(votes, ",", ""))
	r.Metascore, _ = strconv.Atoi(meta)
	ratings(func(src, val string) {
		if src == "Rotten Tomatoes" {
			if n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(val), "%")); err == nil && n >= 0 && n <= 100 {
				r.RottenTomatoes = n
			}
		}
	})
	if r.IMDb < 0 || r.IMDb > 10 {
		r.IMDb = 0
	}
	if r.Metascore < 0 || r.Metascore > 100 {
		r.Metascore = 0
	}
	return r
}
