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
	"regexp"
	"strconv"
	"strings"
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
	HDDs          []HDD      `xml:"e2hdds>e2hdd" json:"hdds"`
}

// Frontend is one tuner.
type Frontend struct {
	Name  string `xml:"e2name" json:"name"`
	Model string `xml:"e2model" json:"model"`
}

// HDD is one storage device from /web/deviceinfo. Capacity and Free are
// human-formatted strings as WebControl reports them (e.g. "2 TB",
// "263.947 GB") — there's no machine-readable byte count in this endpoint
// (confirmed live, ARCHITECTURE.md). See ParseSize for turning these into a
// usage percentage.
type HDD struct {
	Model    string `xml:"e2model" json:"model"`
	Capacity string `xml:"e2capacity" json:"capacity"`
	Free     string `xml:"e2free" json:"free"`
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

// Service is one bouquet or channel entry from /web/getservices (see
// ARCHITECTURE.md). With no query string it lists bouquets; with
// ?sRef=<bouquet reference> it lists the channels inside that bouquet —
// confirmed live 2026-09-24, same schema either way.
type Service struct {
	ServiceReference string `xml:"e2servicereference" json:"serviceReference"`
	ServiceName      string `xml:"e2servicename" json:"serviceName"`
}

type serviceList struct {
	XMLName  xml.Name  `xml:"e2servicelist"`
	Services []Service `xml:"e2service"`
}

// EPGEvent is one <e2event> from /web/epgmulti (see ARCHITECTURE.md: the
// endpoint takes bRef, a bouquet reference, plus a time/endTime window in
// Unix seconds, and returns every event starting in that window for every
// channel in the bouquet, in chronological order per channel — confirmed
// live 2026-09-24).
type EPGEvent struct {
	ID               int64  `xml:"e2eventid" json:"id"`
	Start            int64  `xml:"e2eventstart" json:"start"` // Unix seconds, UTC
	Duration         int64  `xml:"e2eventduration" json:"duration"`
	Title            string `xml:"e2eventtitle" json:"title"`
	Description      string `xml:"e2eventdescription" json:"description"`
	ServiceReference string `xml:"e2eventservicereference" json:"serviceReference"`
	ServiceName      string `xml:"e2eventservicename" json:"serviceName"`
}

type epgEventList struct {
	XMLName xml.Name   `xml:"e2eventlist"`
	Events  []EPGEvent `xml:"e2event"`
}

// Timer mirrors one <e2timer> of /web/timerlist. Field names confirmed
// against a real live response 2026-09-24 (issue #14); see
// backend/testdata/fixtures/timerlist.xml.
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
	// Bouquets lists the box's bouquets (GuideHub's channel groups).
	Bouquets(ctx context.Context) ([]Service, error)
	// Services lists the channels inside a bouquet (bouquetRef, as returned
	// by Bouquets).
	Services(ctx context.Context, bouquetRef string) ([]Service, error)
	// EPGMulti returns every event starting within [start, end) (Unix
	// seconds) for every channel in a bouquet — the data source for
	// GuideHub's timeline grid.
	EPGMulti(ctx context.Context, bouquetRef string, start, end int64) ([]EPGEvent, error)
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
	body, err := c.get(ctx, "/web/movielist")
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return decodeMovieList(body), nil
}

// decodeMovieList tolerates a malformed/truncated tail — confirmed live on
// the reference device 2026-09-24 (see testdata/fixtures/movielist_malformed.xml):
// /web/movielist can end mid-response with an internal "Processing Failed"
// HTML fragment instead of a properly closed </e2movielist>, rather than
// returning an error status — apparently triggered by one specific
// recording's own metadata confusing WebControl's own response generation,
// not anything malformed in the request. Decoding each <e2movie> one at a
// time and keeping whatever parsed successfully before the first decode
// error means one broken entry doesn't take down the whole list.
func decodeMovieList(r io.Reader) []Movie {
	dec := xml.NewDecoder(r)
	var movies []Movie
	for {
		tok, err := dec.Token()
		if err != nil {
			return movies
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "e2movie" {
			continue
		}
		var m Movie
		if dec.DecodeElement(&m, &se) != nil {
			return movies
		}
		movies = append(movies, m)
	}
}

func (c *HTTPClient) TimerList(ctx context.Context) ([]Timer, error) {
	var list timerList
	if err := c.getXML(ctx, "/web/timerlist", &list); err != nil {
		return nil, err
	}
	return list.Timers, nil
}

func (c *HTTPClient) Bouquets(ctx context.Context) ([]Service, error) {
	var list serviceList
	if err := c.getXML(ctx, "/web/getservices", &list); err != nil {
		return nil, err
	}
	return list.Services, nil
}

func (c *HTTPClient) Services(ctx context.Context, bouquetRef string) ([]Service, error) {
	var list serviceList
	q := url.Values{"sRef": {bouquetRef}}
	if err := c.getXML(ctx, "/web/getservices?"+q.Encode(), &list); err != nil {
		return nil, err
	}
	return list.Services, nil
}

func (c *HTTPClient) EPGMulti(ctx context.Context, bouquetRef string, start, end int64) ([]EPGEvent, error) {
	var list epgEventList
	q := url.Values{
		"bRef":    {bouquetRef},
		"time":    {strconv.FormatInt(start, 10)},
		"endTime": {strconv.FormatInt(end, 10)},
	}
	if err := c.getXML(ctx, "/web/epgmulti?"+q.Encode(), &list); err != nil {
		return nil, err
	}
	return list.Events, nil
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

var sizeRe = regexp.MustCompile(`^([\d.]+)\s*([KMGT]?B)$`)
var sizeUnits = map[string]float64{"B": 1, "KB": 1 << 10, "MB": 1 << 20, "GB": 1 << 30, "TB": 1 << 40}

// ParseSize turns a human-formatted size like "2 TB" or "263.947 GB" (the
// only form /web/deviceinfo's HDD fields come in — see HDD) into a byte
// count. Precision is limited by the source string's own rounding; good
// enough for a usage percentage, not for exact byte accounting.
func ParseSize(s string) (int64, bool) {
	m := sizeRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return int64(n * sizeUnits[m[2]]), true
}

// get issues a GET and returns the response body for the caller to read and
// close — the shared HTTP mechanics behind both getXML and MovieList's own
// tolerant decoding.
func (c *HTTPClient) get(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("webcontrol: building request for %s: %w", path, err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webcontrol: calling %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("webcontrol: %s returned status %d", path, resp.StatusCode)
	}
	return resp.Body, nil
}

func (c *HTTPClient) getXML(ctx context.Context, path string, v any) error {
	body, err := c.get(ctx, path)
	if err != nil {
		return err
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("webcontrol: reading %s response: %w", path, err)
	}
	if err := xml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("webcontrol: parsing %s response: %w", path, err)
	}
	return nil
}
