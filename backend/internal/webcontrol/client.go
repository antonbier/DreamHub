// Package webcontrol talks to a Dreambox's WebControl `/web/*` API — the
// confirmed OpenWebif-compatible XML API documented in ARCHITECTURE.md.
// DreamHub calls this instead of re-parsing recordings/EPG files directly,
// per CLAUDE.md's "don't re-implement what WebControl already exposes".
package webcontrol

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DeviceInfo mirrors the confirmed /web/deviceinfo response shape (see
// ARCHITECTURE.md). Only the fields DreamHub actually uses are mapped;
// encoding/xml ignores anything else in the response.
type DeviceInfo struct {
	XMLName       xml.Name   `xml:"e2deviceinfo" json:"-"`
	EnigmaVersion string     `xml:"e2enigmaversion" json:"enigmaVersion"`
	ImageVersion  string     `xml:"e2imageversion" json:"imageVersion"`
	WebifVersion  string     `xml:"e2webifversion" json:"webifVersion"`
	DeviceName    string     `xml:"e2devicename" json:"deviceName"`
	Frontends     []Frontend `xml:"e2frontends>e2frontend" json:"frontends"`
}

// Frontend is one tuner.
type Frontend struct {
	Name  string `xml:"e2name" json:"name"`
	Model string `xml:"e2model" json:"model"`
}

// Tuners returns the number of tuners, at least 1.
func (d *DeviceInfo) Tuners() int {
	if d == nil || len(d.Frontends) == 0 {
		return 1
	}
	return len(d.Frontends)
}

// Movie mirrors one <e2movie> entry from /web/movielist (see
// ARCHITECTURE.md for the confirmed live schema).
type Movie struct {
	ServiceReference string `xml:"e2servicereference" json:"serviceReference"`
	Title            string `xml:"e2title" json:"title"`
	Description      string `xml:"e2description" json:"description"`
	DescriptionExt   string `xml:"e2descriptionextended" json:"descriptionExtended"`
	ServiceName      string `xml:"e2servicename" json:"serviceName"`
	Time             int64  `xml:"e2time" json:"time"`
	Length           string `xml:"e2length" json:"length"`
	Filename         string `xml:"e2filename" json:"filename"`
	FileSize         int64  `xml:"e2filesize" json:"fileSize"`
}

type movieList struct {
	XMLName xml.Name `xml:"e2movielist"`
	Movies  []Movie  `xml:"e2movie"`
}

// Timer mirrors one <e2timer> of /web/timerlist. The endpoint was observed
// during the inspection, but its response body wasn't captured — field names
// follow the OpenWebif schema the rest of the API matches, and need
// confirming against real fixtures (issue #14).
type Timer struct {
	ServiceReference string `xml:"e2servicereference" json:"serviceReference"`
	ServiceName      string `xml:"e2servicename" json:"serviceName"`
	Name             string `xml:"e2name" json:"name"`
	Description      string `xml:"e2description" json:"description"`
	Begin            int64  `xml:"e2timebegin" json:"begin"` // Unix seconds, incl. margin
	End              int64  `xml:"e2timeend" json:"end"`     // Unix seconds, incl. margin
	Disabled         bool   `xml:"e2disabled" json:"disabled"`
	// State follows OpenWebif: 0 waiting, 2 running, 3 finished.
	State int `xml:"e2state" json:"state"`
}

type timerList struct {
	XMLName xml.Name `xml:"e2timerlist"`
	Timers  []Timer  `xml:"e2timer"`
}

// NewTimer is a timer to create. Begin and End already include margins.
type NewTimer struct {
	ServiceReference string
	Name             string
	Description      string
	Begin            int64
	End              int64
}

// Client is anything that can answer WebControl API calls — implemented by
// both the real HTTP client below and the mock in ./mock, per
// PRINCIPLES.md's "Mock-first development".
type Client interface {
	DeviceInfo(ctx context.Context) (*DeviceInfo, error)
	MovieList(ctx context.Context) ([]Movie, error)
	TimerList(ctx context.Context) ([]Timer, error)
	AddTimer(ctx context.Context, t NewTimer) error
}

// ErrWritesDisabled is returned by the real client's write operations until
// they're explicitly enabled. No write endpoint was observed during the
// read-only inspection (ARCHITECTURE.md), so DreamHub doesn't send one to a
// real box on the strength of an assumption.
var ErrWritesDisabled = errors.New("webcontrol: write operations are disabled until verified on the device (see #14)")

// HTTPClient calls a real device's WebControl API over HTTP.
type HTTPClient struct {
	BaseURL string
	HTTP    *http.Client
	// AllowWrites enables write operations (e.g. AddTimer).
	AllowWrites bool
}

// NewHTTPClient returns an HTTPClient with a sane default timeout — a
// device on the LAN should answer quickly, and the backend shouldn't hang
// indefinitely if it doesn't (see PRINCIPLES.md, "Resilience when external
// services are unavailable").
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *HTTPClient) DeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	var info DeviceInfo
	if err := c.getXML(ctx, "/web/deviceinfo", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *HTTPClient) MovieList(ctx context.Context) ([]Movie, error) {
	var list movieList
	if err := c.getXML(ctx, "/web/movielist", &list); err != nil {
		return nil, err
	}
	return list.Movies, nil
}

func (c *HTTPClient) TimerList(ctx context.Context) ([]Timer, error) {
	var list timerList
	if err := c.getXML(ctx, "/web/timerlist", &list); err != nil {
		return nil, err
	}
	return list.Timers, nil
}

type simpleResult struct {
	XMLName   xml.Name `xml:"e2simplexmlresult"`
	State     bool     `xml:"e2state"`
	StateText string   `xml:"e2statetext"`
}

// AddTimer creates a recording timer using the OpenWebif-style
// /web/timeradd endpoint. Unverified on DreamOS WebControl — disabled
// unless AllowWrites is set.
func (c *HTTPClient) AddTimer(ctx context.Context, t NewTimer) error {
	if !c.AllowWrites {
		return ErrWritesDisabled
	}
	q := url.Values{
		"sRef":        {t.ServiceReference},
		"begin":       {strconv.FormatInt(t.Begin, 10)},
		"end":         {strconv.FormatInt(t.End, 10)},
		"name":        {t.Name},
		"description": {t.Description},
		"disabled":    {"0"},
		"justplay":    {"0"},
		"afterevent":  {"3"}, // "auto", Enigma2's default
	}
	var res simpleResult
	if err := c.getXML(ctx, "/web/timeradd?"+q.Encode(), &res); err != nil {
		return err
	}
	if !res.State {
		return fmt.Errorf("webcontrol: timeradd rejected: %s", res.StateText)
	}
	return nil
}

func (c *HTTPClient) getXML(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("webcontrol: building request for %s: %w", path, err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("webcontrol: calling %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webcontrol: %s returned status %d", path, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("webcontrol: reading %s response: %w", path, err)
	}
	if err := xml.Unmarshal(body, v); err != nil {
		return fmt.Errorf("webcontrol: parsing %s response: %w", path, err)
	}
	return nil
}
