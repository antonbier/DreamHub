// Package tmdb is the TMDB connector's client: search for a film or series
// by title, look up an episode's title — the data behind MediaHub's "TMDB"
// name suggestion (decision D2) — and the details, genre names and images
// behind enrichment (internal/enrich: posters, ratings, overviews). An
// external service, used only with a key the user configured
// (PRINCIPLES.md: external calls only where a connector explicitly needs
// them).
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// Details search/multi returns anyway, used by enrichment.
	OriginalTitle string  `json:"originalTitle,omitempty"`
	Overview      string  `json:"overview,omitempty"`
	Rating        float64 `json:"rating,omitempty"` // vote_average, 0-10
	Votes         int     `json:"votes,omitempty"`
	GenreIDs      []int   `json:"genreIds,omitempty"`
	PosterPath    string  `json:"posterPath,omitempty"` // e.g. "/abc.jpg"
	BackdropPath  string  `json:"backdropPath,omitempty"`
}

// Client is implemented by the real HTTP client and ./mock.
type Client interface {
	// Search finds films and series by title, best matches first.
	Search(ctx context.Context, apiKey, query, language string) ([]Result, error)
	// EpisodeTitle returns a series episode's title ("" if TMDB has none).
	EpisodeTitle(ctx context.Context, apiKey string, tvID int64, season, episode int, language string) (string, error)
	// Details fetches one film or series by its TMDB ID (kind "movie" or
	// "tv") — for when another source already knows exactly which title
	// it is, so nothing has to be guessed from a search.
	Details(ctx context.Context, apiKey, kind string, id int64, language string) (Result, error)
	// Genres returns TMDB's genre names by ID, movie and TV genres merged.
	Genres(ctx context.Context, apiKey, language string) (map[int]string, error)
	// Image fetches one image from TMDB's image server; size is one of
	// ImageSizes. No key needed. Returns the bytes and their content type.
	Image(ctx context.Context, size, path string) ([]byte, string, error)
}

// ImageSizes are the image widths DreamHub asks for: a row thumbnail, a
// detail poster, a backdrop.
var ImageSizes = map[string]bool{"w92": true, "w185": true, "w342": true, "w780": true}

// maxImageBytes guards against an unexpectedly huge response.
const maxImageBytes = 5 << 20

var (
	ErrNotConfigured = errors.New("tmdb: no API key configured")
	ErrUnauthorized  = errors.New("tmdb: API key rejected")
	errNotFound      = errors.New("tmdb: not found")
)

// HTTPClient calls api.themoviedb.org.
type HTTPClient struct {
	HTTP    *http.Client
	BaseURL string // overridable in tests

	ImageBaseURL string // overridable in tests
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		HTTP: &http.Client{Timeout: 10 * time.Second}, BaseURL: "https://api.themoviedb.org/3",
		ImageBaseURL: "https://image.tmdb.org/t/p",
	}
}

func (c *HTTPClient) Search(ctx context.Context, apiKey, query, language string) ([]Result, error) {
	var res struct {
		Results []struct {
			MediaType     string  `json:"media_type"`
			ID            int64   `json:"id"`
			Title         string  `json:"title"`
			Name          string  `json:"name"`
			OriginalTitle string  `json:"original_title"`
			OriginalName  string  `json:"original_name"`
			ReleaseDate   string  `json:"release_date"`
			FirstAirDate  string  `json:"first_air_date"`
			Overview      string  `json:"overview"`
			VoteAverage   float64 `json:"vote_average"`
			VoteCount     int     `json:"vote_count"`
			GenreIDs      []int   `json:"genre_ids"`
			PosterPath    string  `json:"poster_path"`
			BackdropPath  string  `json:"backdrop_path"`
		} `json:"results"`
	}
	q := url.Values{"query": {query}, "include_adult": {"false"}, "language": {language}}
	if err := c.get(ctx, apiKey, "/search/multi", q, &res); err != nil {
		return nil, err
	}
	var out []Result
	for _, r := range res.Results {
		res := Result{
			ID: r.ID, Overview: strings.TrimSpace(r.Overview), Rating: r.VoteAverage, Votes: r.VoteCount,
			GenreIDs: r.GenreIDs, PosterPath: r.PosterPath, BackdropPath: r.BackdropPath,
		}
		switch r.MediaType {
		case "movie":
			res.Kind, res.Title, res.OriginalTitle, res.Year = "movie", r.Title, r.OriginalTitle, year(r.ReleaseDate)
		case "tv":
			res.Kind, res.Title, res.OriginalTitle, res.Year = "tv", r.Name, r.OriginalName, year(r.FirstAirDate)
		default:
			continue // people
		}
		out = append(out, res)
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
func (c *HTTPClient) Details(ctx context.Context, apiKey, kind string, id int64, language string) (Result, error) {
	if kind != "movie" && kind != "tv" {
		return Result{}, fmt.Errorf("tmdb: unknown kind %q", kind)
	}
	var d struct {
		ID            int64   `json:"id"`
		Title         string  `json:"title"`
		Name          string  `json:"name"`
		OriginalTitle string  `json:"original_title"`
		OriginalName  string  `json:"original_name"`
		ReleaseDate   string  `json:"release_date"`
		FirstAirDate  string  `json:"first_air_date"`
		Overview      string  `json:"overview"`
		VoteAverage   float64 `json:"vote_average"`
		VoteCount     int     `json:"vote_count"`
		PosterPath    string  `json:"poster_path"`
		BackdropPath  string  `json:"backdrop_path"`
		Genres        []struct {
			ID int `json:"id"`
		} `json:"genres"`
	}
	if err := c.get(ctx, apiKey, fmt.Sprintf("/%s/%d", kind, id), url.Values{"language": {language}}, &d); err != nil {
		return Result{}, err
	}
	r := Result{Kind: kind, ID: d.ID, Overview: strings.TrimSpace(d.Overview), Rating: d.VoteAverage, Votes: d.VoteCount, PosterPath: d.PosterPath, BackdropPath: d.BackdropPath}
	if kind == "movie" {
		r.Title, r.OriginalTitle, r.Year = d.Title, d.OriginalTitle, year(d.ReleaseDate)
	} else {
		r.Title, r.OriginalTitle, r.Year = d.Name, d.OriginalName, year(d.FirstAirDate)
	}
	for _, g := range d.Genres {
		r.GenreIDs = append(r.GenreIDs, g.ID)
	}
	return r, nil
}

func (c *HTTPClient) Genres(ctx context.Context, apiKey, language string) (map[int]string, error) {
	out := map[int]string{}
	for _, kind := range []string{"movie", "tv"} {
		var res struct {
			Genres []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"genres"`
		}
		if err := c.get(ctx, apiKey, "/genre/"+kind+"/list", url.Values{"language": {language}}, &res); err != nil {
			return nil, err
		}
		for _, g := range res.Genres {
			out[g.ID] = g.Name
		}
	}
	return out, nil
}

// validImagePath accepts only TMDB's own "/<id>.<ext>" image paths, so the
// image proxy can't be pointed anywhere else.
func validImagePath(p string) bool {
	if len(p) < 6 || len(p) > 64 || p[0] != '/' {
		return false
	}
	name := p[1:]
	dot := strings.LastIndexByte(name, '.')
	if dot <= 0 {
		return false
	}
	switch name[dot+1:] {
	case "jpg", "png", "webp":
	default:
		return false
	}
	for _, r := range name[:dot] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// ErrInvalidImage is returned by Image for a size or path it won't fetch.
var ErrInvalidImage = errors.New("tmdb: invalid image size or path")

func (c *HTTPClient) Image(ctx context.Context, size, path string) ([]byte, string, error) {
	if !ImageSizes[size] || !validImagePath(path) {
		return nil, "", ErrInvalidImage
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ImageBaseURL+"/"+size+path, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("tmdb: fetching image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("tmdb: image returned status %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return nil, "", fmt.Errorf("tmdb: image has content type %q", ct)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(b) > maxImageBytes {
		return nil, "", fmt.Errorf("tmdb: image larger than %d bytes", maxImageBytes)
	}
	return b, ct, nil
}

// ValidImage reports whether Image would accept size and path.
func ValidImage(size, path string) bool { return ImageSizes[size] && validImagePath(path) }

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
