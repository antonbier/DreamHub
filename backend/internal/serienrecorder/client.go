// Package serienrecorder talks to the SerienRecorder plugin's own web API,
// confirmed live 2026-09-24 (ARCHITECTURE.md, issue #10): reachable at
// /serienrecorderapi/*, on the same host and port as WebControl but
// returning JSON, not XML — the one exception in this codebase. SeriesHub
// calls this instead of reading SerienRecorder.db directly, since that file
// may be being written to by the plugin itself (ROADMAP.md).
package serienrecorder

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

// Marker is one tracked series (an "e2movie"-scale unit for SeriesHub).
// Field names and casing (fsid, lowercase) mirror the real API exactly —
// confirmed live, see backend/testdata/fixtures/serienrecorder/markers.json.
type Marker struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	FSID            string `json:"fsid"`
	WLID            string `json:"wlid"`
	Active          bool   `json:"active"`
	Info            string `json:"info"`
	Channels        string `json:"channels"`
	NumberOfRecords int    `json:"numberOfRecords"`
	// Seasons is SerienRecorder's own description of which seasons it
	// records ("Alle (inkl. Specials)"), RecordFolder where the recordings
	// go (confirmed in the live markers fixture).
	Seasons      string `json:"seasons"`
	RecordFolder string `json:"recordfolder"`
}

// UpcomingEpisode is one entry from /serienrecorderapi/timer?remaining=1 —
// an episode SerienRecorder will record. Field casing (fsID, capital ID)
// differs from Marker.FSID; both mirror the real API exactly.
type UpcomingEpisode struct {
	Series           string `json:"series"`
	Title            string `json:"title"`
	Channel          string `json:"channel"`
	ServiceReference string `json:"serviceRef"`
	Season           string `json:"season"`
	Episode          string `json:"episode"`
	StartTime        int64  `json:"startTime"` // Unix seconds, UTC
	FSID             string `json:"fsID"`
}

// Info is /serienrecorderapi/info — used only as a lightweight reachability
// check (connector.SerienRecorder).
type Info struct {
	APIVersion string `json:"apiVersion"`
	SRVersion  string `json:"srVersion"`
}

// Client is anything that can answer SerienRecorder API calls — implemented
// by both the real HTTP client below and the mock in ./mock, per
// PRINCIPLES.md's "Mock-first development".
// SeriesInfo is what SerienRecorder knows about a series from its own
// server (/serienrecorderapi/seriesinfo, confirmed live 2026-09-28):
// German description, genres and length, cast, first year ("2018-").
type SeriesInfo struct {
	Title       string   `json:"title"`
	Year        string   `json:"year"`
	Info        string   `json:"info"` // "Drama, Crime, Action (42 min)"
	Description string   `json:"description"`
	Cast        []string `json:"cast"`
	// "rating" isn't taken over: its value on the reference box (268909)
	// is no rating anyone could read.
}

// StartYear is the series' first year, 0 when unknown.
func (s SeriesInfo) StartYear() int {
	if len(s.Year) < 4 {
		return 0
	}
	y, err := strconv.Atoi(s.Year[:4])
	if err != nil {
		return 0
	}
	return y
}

type Client interface {
	Info(ctx context.Context) (*Info, error)
	Markers(ctx context.Context) ([]Marker, error)
	UpcomingEpisodes(ctx context.Context) ([]UpcomingEpisode, error)
	// SeriesInfo is nil (no error) when SerienRecorder has nothing.
	SeriesInfo(ctx context.Context, wlid, fsid string) (*SeriesInfo, error)
}

// HTTPClient calls a real device's SerienRecorder API over HTTP. BaseURL is
// the same device URL WebControl uses (e.g. http://192.168.1.43) — the API
// lives on the same host, just a different path.
type HTTPClient struct {
	BaseURL string
	HTTP    *http.Client
	// Credentials, if set, supplies Basic Auth — the API sits behind
	// WebControl's own auth, so it's the same login (connector settings,
	// "webcontrol").
	Credentials func() (user, password string)
}

// ErrNotInstalled means the box answered, but has no SerienRecorder API:
// the plugin isn't installed (or its web interface is switched off). An
// optional plugin missing is a normal state, not a broken connection.
var ErrNotInstalled = errors.New("serienrecorder: API not found on the box (plugin not installed or web interface disabled)")

// ErrUnauthorized means WebControl, which the API sits behind, asked for
// credentials.
var ErrUnauthorized = errors.New("serienrecorder: credentials required or rejected")

// NewHTTPClient returns an HTTPClient with the same timeout convention as
// webcontrol.NewHTTPClient (see its comment).
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{BaseURL: baseURL, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

type infoEnvelope struct {
	Payload Info `json:"payload"`
}

func (c *HTTPClient) Info(ctx context.Context) (*Info, error) {
	var env infoEnvelope
	if err := c.getJSON(ctx, "/serienrecorderapi/info", &env); err != nil {
		return nil, err
	}
	return &env.Payload, nil
}

type markersEnvelope struct {
	Payload struct {
		Markers []Marker `json:"markers"`
	} `json:"payload"`
}

func (c *HTTPClient) Markers(ctx context.Context) ([]Marker, error) {
	var env markersEnvelope
	if err := c.getJSON(ctx, "/serienrecorderapi/markers", &env); err != nil {
		return nil, err
	}
	return env.Payload.Markers, nil
}

type timerEnvelope struct {
	Payload struct {
		Timer []UpcomingEpisode `json:"timer"`
	} `json:"payload"`
}

func (c *HTTPClient) UpcomingEpisodes(ctx context.Context) ([]UpcomingEpisode, error) {
	var env timerEnvelope
	q := url.Values{"remaining": {"1"}}
	if err := c.getJSON(ctx, "/serienrecorderapi/timer?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	return env.Payload.Timer, nil
}

type seriesInfoEnvelope struct {
	Payload struct {
		Info *SeriesInfo `json:"info"`
	} `json:"payload"`
}

func (c *HTTPClient) SeriesInfo(ctx context.Context, wlid, fsid string) (*SeriesInfo, error) {
	if wlid == "" || fsid == "" {
		return nil, nil // the endpoint answers an empty payload without both
	}
	var env seriesInfoEnvelope
	q := url.Values{"wlid": {wlid}, "fsid": {fsid}}
	if err := c.getJSON(ctx, "/serienrecorderapi/seriesinfo?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	if env.Payload.Info == nil || env.Payload.Info.Title == "" {
		return nil, nil
	}
	return env.Payload.Info, nil
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("serienrecorder: building request for %s: %w", path, err)
	}
	if c.Credentials != nil {
		if user, pass := c.Credentials(); user != "" {
			req.SetBasicAuth(user, pass)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("serienrecorder: calling %s: %w", path, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return ErrNotInstalled
	case http.StatusUnauthorized:
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("serienrecorder: %s returned status %d", path, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("serienrecorder: reading %s response: %w", path, err)
	}
	// A missing/invalid required parameter makes this API return an HTML
	// error page instead of JSON (confirmed live, ARCHITECTURE.md) — surface
	// that as a clear error rather than a cryptic JSON-parse failure.
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("serienrecorder: parsing %s response (got non-JSON body?): %w", path, err)
	}
	return nil
}
