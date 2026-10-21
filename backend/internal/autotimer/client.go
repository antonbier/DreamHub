// Package autotimer talks to the AutoTimer plugin's own web API
// (`/autotimer`, XML — see ARCHITECTURE.md, "AutoTimer web API"). The plugin
// is optional: on a box without it the API answers 404, reported as
// ErrNotInstalled, which is a normal state (docs/archive/V1_PLAN.md, rule 1).
//
// DreamHub only reads here: the configured AutoTimers and a dry run of what
// they would create (`/autotimer/simulate`, confirmed side-effect free:
// parseEPG(simulateOnly=True)). Editing stays in the plugin's own web GUI
// (`/autotimereditor/`), linked from TimerHub — see decision D3.
package autotimer

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AutoTimer is one configured AutoTimer (a subset of its attributes).
type AutoTimer struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	Match    string    `json:"match"`
	Enabled  bool      `json:"enabled"`
	Services []Service `json:"services"`
}

// Service is a channel an AutoTimer is limited to.
type Service struct {
	ServiceReference string `json:"serviceReference"`
	ServiceName      string `json:"serviceName"`
}

// SimulatedTimer is a timer the AutoTimers would create right now.
type SimulatedTimer struct {
	ServiceReference string `json:"serviceReference"`
	ServiceName      string `json:"serviceName"`
	Name             string `json:"name"`
	Begin            int64  `json:"begin"` // Unix seconds, UTC
	End              int64  `json:"end"`
	AutoTimerName    string `json:"autoTimerName"`
}

// Client is implemented by the real HTTP client and ./mock.
type Client interface {
	List(ctx context.Context) ([]AutoTimer, error)
	Simulate(ctx context.Context) ([]SimulatedTimer, error)
}

var (
	// ErrNotInstalled: the box has no AutoTimer web API.
	ErrNotInstalled = errors.New("autotimer: API not found on the box (plugin not installed)")
	// ErrUnauthorized: WebControl, which the API sits behind, asked for
	// credentials.
	ErrUnauthorized = errors.New("autotimer: credentials required or rejected")
)

// HTTPClient calls a real box. BaseURL is WebControl's.
type HTTPClient struct {
	BaseURL     string
	HTTP        *http.Client
	Credentials func() (user, password string)
}

// NewHTTPClient returns a client with a timeout. Simulate runs the plugin's
// EPG parsing, so it gets more time than a plain read.
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{BaseURL: baseURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type listXML struct {
	Timers []struct {
		ID       int64  `xml:"id,attr"`
		Name     string `xml:"name,attr"`
		Match    string `xml:"match,attr"`
		Enabled  string `xml:"enabled,attr"`
		Services []struct {
			Ref  string `xml:"e2servicereference"`
			Name string `xml:"e2servicename"`
		} `xml:"e2service"`
	} `xml:"timer"`
}

func (c *HTTPClient) List(ctx context.Context) ([]AutoTimer, error) {
	var raw listXML
	if err := c.getXML(ctx, "/autotimer", &raw); err != nil {
		return nil, err
	}
	out := make([]AutoTimer, 0, len(raw.Timers))
	for _, t := range raw.Timers {
		at := AutoTimer{ID: t.ID, Name: t.Name, Match: t.Match, Enabled: t.Enabled == "yes", Services: []Service{}}
		for _, s := range t.Services {
			at.Services = append(at.Services, Service{ServiceReference: s.Ref, ServiceName: s.Name})
		}
		out = append(out, at)
	}
	return out, nil
}

type simulateXML struct {
	Timers []struct {
		Ref           string `xml:"e2servicereference"`
		ServiceName   string `xml:"e2servicename"`
		Name          string `xml:"e2name"`
		Begin         int64  `xml:"e2timebegin"`
		End           int64  `xml:"e2timeend"`
		AutoTimerName string `xml:"e2autotimername"`
	} `xml:"e2simulatedtimer"`
}

func (c *HTTPClient) Simulate(ctx context.Context) ([]SimulatedTimer, error) {
	var raw simulateXML
	if err := c.getXML(ctx, "/autotimer/simulate", &raw); err != nil {
		return nil, err
	}
	out := make([]SimulatedTimer, 0, len(raw.Timers))
	for _, t := range raw.Timers {
		out = append(out, SimulatedTimer{ServiceReference: t.Ref, ServiceName: t.ServiceName, Name: t.Name, Begin: t.Begin, End: t.End, AutoTimerName: t.AutoTimerName})
	}
	return out, nil
}

func (c *HTTPClient) getXML(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("autotimer: building request for %s: %w", path, err)
	}
	if c.Credentials != nil {
		if user, pass := c.Credentials(); user != "" {
			req.SetBasicAuth(user, pass)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("autotimer: calling %s: %w", path, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return ErrNotInstalled
	case http.StatusUnauthorized:
		return ErrUnauthorized
	default:
		return fmt.Errorf("autotimer: %s returned status %d", path, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("autotimer: reading %s: %w", path, err)
	}
	// OpenWebif (VTi 15, 2026-10-21) answers /autotimer with 200 and this
	// stub when the plugin is missing; the real sub-paths are then 404.
	if bytes.Contains(data, []byte("AutoTimer Plugin not found")) {
		return ErrNotInstalled
	}
	if err := xml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("autotimer: parsing %s: %w", path, err)
	}
	return nil
}
