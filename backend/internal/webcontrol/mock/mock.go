// Package mock implements webcontrol.Client against fixture data instead of
// a real device, so the rest of DreamHub is buildable and testable without
// a reachable Dreambox. See PRINCIPLES.md, "Mock-first development", and
// ROADMAP.md, Foundation.
//
// Device info and the recording are taken directly from the live responses
// recorded in ARCHITECTURE.md. The two timers are the example evening used
// in the design mockup (docs/design/): "Bauer sucht Frau International" and
// an overlapping "The Rookie" — a real conflict on a single-tuner box.
package mock

import (
	"context"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Client is a webcontrol.Client backed by in-memory fixture data. Timers
// are stateful, so AddTimer is visible in later TimerList calls.
type Client struct {
	mu     sync.Mutex
	timers []webcontrol.Timer
}

var _ webcontrol.Client = (*Client)(nil)

// Evening is the day the fixture timers are on (2026-09-23, Europe/Rome).
var Evening = time.Date(2026, 9, 23, 0, 0, 0, 0, time.FixedZone("CEST", 2*60*60))

func at(h, m int) int64 {
	return Evening.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).Unix()
}

// New returns a mock with the fixture timers.
func New() *Client {
	return &Client{timers: []webcontrol.Timer{
		{
			ServiceReference: "1:0:19:2EE3:441:1:C00000:0:0:0:",
			ServiceName:      "RTL Television",
			Name:             "Bauer sucht Frau International",
			Begin:            at(20, 15),
			End:              at(21, 15),
		},
		{
			ServiceReference: "1:0:19:2B7A:3F3:1:C00000:0:0:0:",
			ServiceName:      "zdf_neo HD",
			Name:             "The Rookie",
			Begin:            at(21, 0),
			End:              at(21, 45),
		},
	}}
}

func (*Client) DeviceInfo(ctx context.Context) (*webcontrol.DeviceInfo, error) {
	return &webcontrol.DeviceInfo{
		EnigmaVersion: "5.0.0r117-gutemine-2024-08-12",
		ImageVersion:  "Experimental 2023-04-24",
		WebifVersion:  "1.9.1",
		DeviceName:    "one",
		Frontends:     []webcontrol.Frontend{{Name: "Tuner A", Model: " Si2169D (DVB-S2 / DVB-C / DVB-T2)"}},
		// Real capture, 2026-09-24 (ARCHITECTURE.md, #14): the reference
		// device's disk really is close to full — a genuine, recurring pain
		// point MediaHub's disk-space overview exists to surface.
		HDDs: []webcontrol.HDD{{Model: "ST2000LM 015-2E8174", Capacity: "2 TB", Free: "263.947 GB"}},
	}, nil
}

func (*Client) MovieList(ctx context.Context) ([]webcontrol.Movie, error) {
	return []webcontrol.Movie{
		{
			ServiceReference: `1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/20260608 2048 - RTL Television - Bauer sucht Frau International.ts`,
			Title:            "Bauer sucht Frau International",
			Description:      "Folge 7",
			ServiceName:      "RTL Television",
			Time:             1780944521,
			Length:           "103:16",
			Filename:         `//media/hdd/movie/20260608 2048 - RTL Television - Bauer sucht Frau International.ts`,
			FileSize:         3802689536,
		},
		// A manually-copied, non-recording file in the same folder (real,
		// confirmed live, ARCHITECTURE.md's MediaHub note): the scanner must
		// leave these alone rather than treating them as recordings.
		{
			ServiceReference: `1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/Oskars Kleid.mkv`,
			Title:            "Oskars Kleid.mkv",
			Filename:         `//media/hdd/movie/Oskars Kleid.mkv`,
			FileSize:         1200000000,
		},
	}, nil
}

func (c *Client) TimerList(ctx context.Context) ([]webcontrol.Timer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]webcontrol.Timer, len(c.timers))
	copy(out, c.timers)
	return out, nil
}

// Real bouquet/channel references captured live 2026-09-24 (ARCHITECTURE.md).
var favouritesTV = webcontrol.Service{
	ServiceReference: `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`,
	ServiceName:      "Free TV",
}

func (*Client) Bouquets(ctx context.Context) ([]webcontrol.Service, error) {
	return []webcontrol.Service{favouritesTV}, nil
}

func (*Client) Services(ctx context.Context, bouquetRef string) ([]webcontrol.Service, error) {
	return []webcontrol.Service{
		{ServiceReference: "1:0:19:2EE3:441:1:C00000:0:0:0:", ServiceName: "RTL Television"},
		{ServiceReference: "1:0:19:2B7A:3F3:1:C00000:0:0:0:", ServiceName: "zdf_neo HD"},
	}, nil
}

func (c *Client) EPGNowNext(ctx context.Context, bouquetRef string) ([]webcontrol.EPGEvent, error) {
	return []webcontrol.EPGEvent{
		{ID: 1, Start: at(19, 40), Duration: 35 * 60, Title: "RTL Aktuell", ServiceReference: "1:0:19:2EE3:441:1:C00000:0:0:0:", ServiceName: "RTL Television"},
		{ID: 2, Start: at(20, 15), Duration: 60 * 60, Title: "Bauer sucht Frau International", ServiceReference: "1:0:19:2EE3:441:1:C00000:0:0:0:", ServiceName: "RTL Television"},
		{ID: 3, Start: at(19, 45), Duration: 75 * 60, Title: "heute-show", ServiceReference: "1:0:19:2B7A:3F3:1:C00000:0:0:0:", ServiceName: "zdf_neo HD"},
		{ID: 4, Start: at(21, 0), Duration: 45 * 60, Title: "The Rookie", ServiceReference: "1:0:19:2B7A:3F3:1:C00000:0:0:0:", ServiceName: "zdf_neo HD"},
	}, nil
}

func (c *Client) AddTimer(ctx context.Context, t webcontrol.NewTimer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timers = append(c.timers, webcontrol.Timer{
		ServiceReference: t.ServiceReference,
		Name:             t.Name,
		Description:      t.Description,
		Begin:            t.Begin,
		End:              t.End,
	})
	return nil
}
