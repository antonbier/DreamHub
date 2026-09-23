// Package webcontrol talks to a Dreambox's WebControl `/web/*` API — the
// confirmed OpenWebif-compatible XML API documented in ARCHITECTURE.md.
// DreamHub calls this instead of re-parsing recordings/EPG files directly,
// per CLAUDE.md's "don't re-implement what WebControl already exposes".
package webcontrol

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeviceInfo mirrors the confirmed /web/deviceinfo response shape (see
// ARCHITECTURE.md). Only the fields DreamHub actually uses are mapped;
// encoding/xml ignores anything else in the response.
type DeviceInfo struct {
	XMLName       xml.Name `xml:"e2deviceinfo" json:"-"`
	EnigmaVersion string   `xml:"e2enigmaversion" json:"enigmaVersion"`
	ImageVersion  string   `xml:"e2imageversion" json:"imageVersion"`
	WebifVersion  string   `xml:"e2webifversion" json:"webifVersion"`
	DeviceName    string   `xml:"e2devicename" json:"deviceName"`
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

// Client is anything that can answer WebControl API calls — implemented by
// both the real HTTP client below and the mock server in ./mock, per
// PRINCIPLES.md's "Mock-first development".
type Client interface {
	DeviceInfo(ctx context.Context) (*DeviceInfo, error)
	MovieList(ctx context.Context) ([]Movie, error)
}

// HTTPClient calls a real device's WebControl API over HTTP.
type HTTPClient struct {
	BaseURL string
	HTTP    *http.Client
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
