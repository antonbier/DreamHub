// Package fanart is the fanart.tv connector's client: a film's or series'
// logo (the title as a transparent picture), which neither the box nor
// TMDB's details carry. Films are looked up by their TMDB ID, series by
// TheTVDB's (TMDB supplies it) — nothing is guessed from a title. Needs a
// free personal API key. Logos are fetched once and kept in DreamHub's
// image cache; the browser never contacts fanart.tv (PRINCIPLES.md).
package fanart

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

// Client is implemented by HTTPClient and ./mock.
type Client interface {
	// Logo returns the best logo's path on fanart.tv's image server
	// ("/fanart/movies/603/hdmovielogo/the-matrix-….png"), "" when there
	// is none. kind "movie" takes a TMDB ID, "tv" a TheTVDB ID; langs are
	// preferred languages in order ("de", "en").
	Logo(ctx context.Context, apiKey, kind string, id int64, langs []string) (string, error)
	// Image fetches one picture by such a path. No key needed.
	Image(ctx context.Context, path string) ([]byte, string, error)
}

var (
	ErrNotConfigured = errors.New("fanart: no API key configured")
	ErrUnauthorized  = errors.New("fanart: API key rejected")
	ErrInvalidImage  = errors.New("fanart: invalid image path")
)

var validPath = regexp.MustCompile(`^/fanart/(movies|tv)/\d{1,10}/[a-z]{3,20}/[A-Za-z0-9._-]{1,120}\.(png|jpg)$`)

// ValidImage accepts only fanart.tv's own image paths, so the image proxy
// can't be pointed anywhere else.
func ValidImage(path string) bool {
	return validPath.MatchString(path) && !strings.Contains(path, "..")
}

// HTTPClient calls webservice.fanart.tv and assets.fanart.tv.
type HTTPClient struct {
	HTTP         *http.Client
	BaseURL      string // overridable in tests
	ImageBaseURL string
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{HTTP: &http.Client{Timeout: 10 * time.Second}, BaseURL: "https://webservice.fanart.tv/v3", ImageBaseURL: "https://assets.fanart.tv"}
}

type art struct {
	URL   string `json:"url"`
	Lang  string `json:"lang"`
	Likes string `json:"likes"`
}

func (c *HTTPClient) Logo(ctx context.Context, apiKey, kind string, id int64, langs []string) (string, error) {
	if apiKey == "" {
		return "", ErrNotConfigured
	}
	var section string
	var kinds []string // HD first
	switch kind {
	case "movie":
		section, kinds = "movies", []string{"hdmovielogo", "movielogo"}
	case "tv":
		section, kinds = "tv", []string{"hdtvlogo", "clearlogo"}
	default:
		return "", fmt.Errorf("fanart: unknown kind %q", kind)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/"+section+"/"+strconv.FormatInt(id, 10)+"?"+url.Values{"api_key": {apiKey}}.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", errors.New("fanart: request failed") // never the URL: it carries the key
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", ErrUnauthorized
	default:
		return "", fmt.Errorf("fanart: status %d", resp.StatusCode)
	}
	var d map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&d); err != nil {
		return "", err
	}
	for _, k := range kinds {
		var arts []art
		if json.Unmarshal(d[k], &arts) != nil {
			continue
		}
		if p := best(arts, langs); p != "" {
			return p, nil
		}
	}
	return "", nil
}

// best picks the most-liked logo in the first preferred language that has
// one, then a language-neutral one ("00"), then any; returned as a path
// on the image server.
func best(arts []art, langs []string) string {
	order := append(append([]string{}, langs...), "00", "")
	for _, lang := range order {
		bestLikes, bestPath := -1, ""
		for _, a := range arts {
			if lang != "" && a.Lang != lang {
				continue
			}
			u, err := url.Parse(a.URL)
			if err != nil || u.Host != "assets.fanart.tv" || !ValidImage(u.Path) {
				continue
			}
			likes, _ := strconv.Atoi(a.Likes)
			if likes > bestLikes {
				bestLikes, bestPath = likes, u.Path
			}
		}
		if bestPath != "" {
			return bestPath
		}
	}
	return ""
}

// maxImageBytes guards against an unexpectedly huge picture (an HD logo
// is ~100 KB).
const maxImageBytes = 5 << 20

func (c *HTTPClient) Image(ctx context.Context, path string) ([]byte, string, error) {
	if !ValidImage(path) {
		return nil, "", ErrInvalidImage
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ImageBaseURL+path, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fanart: image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fanart: image status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(b) > maxImageBytes {
		return nil, "", errors.New("fanart: image too large")
	}
	ct := http.DetectContentType(b)
	if ct != "image/png" && ct != "image/jpeg" {
		return nil, "", errors.New("fanart: not an image")
	}
	return b, ct, nil
}
