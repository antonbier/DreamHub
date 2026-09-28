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
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Client is a webcontrol.Client backed by in-memory fixture data. Timers
// are stateful, so AddTimer is visible in later TimerList calls, and the
// recordings live in FS — the same in-memory file system MediaHub's file
// operations work on in mock mode, so a rename shows up in the next
// listing exactly as on a real box (ARCHITECTURE.md, #11).
type Client struct {
	mu     sync.Mutex
	timers []webcontrol.Timer
	// FS holds the recordings (see Root).
	FS *recfs.Mem
	// Playing is what /web/getcurrent reports; a recording's reference
	// ("1:0:0:0:0:0:0:0:0:0:/path.ts") marks that file as in use.
	Playing string
	// Standby is what InStandby reports (ControlHub).
	Standby bool
	// LastZapped and LastKey record the most recent ControlHub action, for
	// tests to assert on.
	LastZapped string
	LastKey    int
	// screen is the simulated on-screen state Screenshot renders (screen.go).
	screen screen
}

// Root is the mock box's recording folder.
const Root = "/media/hdd/movie/"

// recordingFixtures is the mock box's disk: a recording with its full
// sidecar set (the #11 live test's set), a manually copied non-recording,
// a SerienRecorder-style subfolder and one recording already in the trash
// (AdvancedMovieSelection's ".ts.trash" convention).
func recordingFixtures() map[string]string {
	bauer := Root + "20260608 2048 - RTL Television - Bauer sucht Frau International"
	tatort := Root + "Tatort/20260920 2015 - Das Erste HD - Tatort_ Borowski und das Haus am Meer"
	old := Root + "20250101 2015 - ZDF - Alte Doku"
	return map[string]string{
		bauer + ".ts":             "recording",
		bauer + ".ts.meta":        "1:0:19:2EE3:441:1:C00000:0:0:0:\nBauer sucht Frau International\nFolge 7\n1780944521\n",
		bauer + ".ts.ap":          "ap",
		bauer + ".ts.sc":          "sc",
		bauer + ".ts.cuts":        "cuts",
		bauer + ".eit":            "eit",
		bauer + ".jpg":            "cover",
		Root + "Oskars Kleid.mkv": "not a recording",
		tatort + ".ts":            "recording",
		tatort + ".ts.meta":       "1:0:19:283D:3FB:1:C00000:0:0:0:\nTatort: Borowski und das Haus am Meer\n\n1790021700\n",
		tatort + ".eit":           "eit",
		old + ".ts.trash":         "recording",
		old + ".ts.meta":          "1:0:19:2B66:3F2:1:C00000:0:0:0:\nAlte Doku\n\n1735758900\n",
	}
}

var _ webcontrol.Client = (*Client)(nil)

// Evening is the day the fixture timers are on (2026-09-23, Europe/Rome).
var Evening = time.Date(2026, 9, 23, 0, 0, 0, 0, time.FixedZone("CEST", 2*60*60))

func at(h, m int) int64 {
	return Evening.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).Unix()
}

// New returns a mock with the fixture timers.
func New() *Client {
	return &Client{FS: recfs.NewMem(recordingFixtures()), timers: []webcontrol.Timer{
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

// epgSchedule is a full evening's worth of events for the mock's 2
// channels, so switching the guide's time window actually shows different
// programming — not just the 2 events an earlier now/next-only mock had.
// "Bauer sucht Frau International" and "The Rookie" deliberately match the
// two fixture timers above (same channel, same times): the guide and
// TimerHub tell a consistent story about the same evening.
func epgSchedule() []webcontrol.EPGEvent {
	rtl := webcontrol.Service{ServiceReference: "1:0:19:2EE3:441:1:C00000:0:0:0:", ServiceName: "RTL Television"}
	zdfneo := webcontrol.Service{ServiceReference: "1:0:19:2B7A:3F3:1:C00000:0:0:0:", ServiceName: "zdf_neo HD"}
	ard := webcontrol.Service{ServiceReference: "1:0:19:283D:3FB:1:C00000:0:0:0:", ServiceName: "Das Erste HD"}
	return []webcontrol.EPGEvent{
		{ID: 1, Start: at(19, 5), Duration: 35 * 60, Title: "Explosiv – Das Magazin", ServiceReference: rtl.ServiceReference, ServiceName: rtl.ServiceName},
		{ID: 2, Start: at(19, 40), Duration: 35 * 60, Title: "RTL Aktuell", ServiceReference: rtl.ServiceReference, ServiceName: rtl.ServiceName},
		{ID: 3, Start: at(20, 15), Duration: 60 * 60, Title: "Bauer sucht Frau International", ServiceReference: rtl.ServiceReference, ServiceName: rtl.ServiceName},
		{ID: 4, Start: at(21, 15), Duration: 60 * 60, Title: "Undercover Boss", ServiceReference: rtl.ServiceReference, ServiceName: rtl.ServiceName},
		{ID: 5, Start: at(22, 15), Duration: 60 * 60, Title: "stern TV", ServiceReference: rtl.ServiceReference, ServiceName: rtl.ServiceName},
		{ID: 6, Start: at(18, 30), Duration: 75 * 60, Title: "Notruf Hafenkante", ServiceReference: zdfneo.ServiceReference, ServiceName: zdfneo.ServiceName},
		{ID: 7, Start: at(19, 45), Duration: 75 * 60, Title: "heute-show", ServiceReference: zdfneo.ServiceReference, ServiceName: zdfneo.ServiceName},
		{ID: 8, Start: at(21, 0), Duration: 45 * 60, Title: "The Rookie", ServiceReference: zdfneo.ServiceReference, ServiceName: zdfneo.ServiceName},
		{ID: 9, Start: at(21, 45), Duration: 45 * 60, Title: "The Rookie: Feds", ServiceReference: zdfneo.ServiceReference, ServiceName: zdfneo.ServiceName},
		{ID: 10, Start: at(22, 30), Duration: 60 * 60, Title: "Bones", ServiceReference: zdfneo.ServiceReference, ServiceName: zdfneo.ServiceName},
		{ID: 11, Start: at(20, 15), Duration: 90 * 60, Title: "Tatort: Borowski und der gute Mensch", Description: "Kriminalfilm, D 2026", ServiceReference: ard.ServiceReference, ServiceName: ard.ServiceName},
		{ID: 12, Start: at(21, 45), Duration: 30 * 60, Title: "Tatort-Reiniger Spezial", ServiceReference: ard.ServiceReference, ServiceName: ard.ServiceName},
	}
}

func (c *Client) EPGMulti(ctx context.Context, bouquetRef string, start, end int64) ([]webcontrol.EPGEvent, error) {
	var out []webcontrol.EPGEvent
	for _, e := range epgSchedule() {
		if e.Start >= start && e.Start < end {
			out = append(out, e)
		}
	}
	return out, nil
}

func (c *Client) EPGSearch(ctx context.Context, query string) ([]webcontrol.EPGEvent, error) {
	q := strings.ToLower(query)
	var out []webcontrol.EPGEvent
	for _, e := range epgSchedule() {
		if strings.Contains(strings.ToLower(e.Title), q) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (c *Client) Locations(ctx context.Context) ([]string, error) {
	return []string{Root}, nil
}

// MovieDir lists a folder of FS like /web/movielist?dirname= does: .ts and
// other media files (not sidecars, not trashed recordings), plus the direct
// subfolders. Titles come from .ts.meta, as on the box.
func (c *Client) MovieDir(ctx context.Context, dir string) (webcontrol.MovieFolder, error) {
	f := webcontrol.MovieFolder{Movies: []webcontrol.Movie{}, Subfolders: []string{}}
	entries, err := c.FS.ReadDir(dir)
	if err != nil {
		return f, nil // the box answers an empty list for a missing folder
	}
	base := strings.TrimSuffix(dir, "/") + "/"
	for _, e := range entries {
		full := base + e.Name()
		if e.IsDir() {
			f.Subfolders = append(f.Subfolders, full+"/")
			continue
		}
		if !strings.HasSuffix(e.Name(), ".ts") && !strings.HasSuffix(e.Name(), ".mkv") {
			continue
		}
		m := webcontrol.Movie{
			ServiceReference: "1:0:0:0:0:0:0:0:0:0:" + full,
			Title:            e.Name(),
			Filename:         "/" + full,
			FileSize:         webcontrol.Int64(c.FS.Size(full)),
		}
		if title := recfs.MetaTitle(c.FS, full); title != "" {
			m.Title = title
		}
		f.Movies = append(f.Movies, m)
	}
	return f, nil
}

func (c *Client) CurrentServiceReference(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Playing != "" {
		return c.Playing, nil
	}
	return "1:0:19:2EE3:441:1:C00000:0:0:0:", nil // RTL, live TV
}

// Zap records the requested channel as playing, like a real zap would.
func (c *Client) Zap(ctx context.Context, serviceReference string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastZapped = serviceReference
	c.Playing = serviceReference
	return nil
}

// SendKey records the code and applies it to the simulated screen
// (screen.go), so a key press shows up in the next Screenshot.
func (c *Client) SendKey(ctx context.Context, code int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastKey = code
	c.screen.press(code, &c.Standby)
	return nil
}

// Screenshot renders the simulated screen: a test card, the menu overlay
// when MENU opened it, black in standby.
func (c *Client) Screenshot(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.screen.render(c.Standby)
}

func (c *Client) InStandby(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Standby, nil
}

// AddTimer behaves like the real box (ARCHITECTURE.md, "Timer write
// endpoints"), including its quirk of reporting success for a timer it
// silently drops because one with the same service and begin already
// exists — so callers' verify-after-write logic is exercised in tests too.
func (c *Client) AddTimer(ctx context.Context, t webcontrol.NewTimer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.timers {
		if existing.ServiceReference == t.ServiceReference && existing.Begin == t.Begin {
			return nil
		}
	}
	c.timers = append(c.timers, webcontrol.Timer{
		ServiceReference: t.ServiceReference,
		Name:             t.Name,
		Description:      t.Description,
		Begin:            t.Begin,
		End:              t.End,
	})
	return nil
}

// DeleteTimer matches exactly like the real box and answers the same way
// when nothing matches.
func (c *Client) DeleteTimer(ctx context.Context, ref webcontrol.TimerRef) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, t := range c.timers {
		if t.ServiceReference == ref.ServiceReference && t.Begin == ref.Begin && t.End == ref.End {
			c.timers = append(c.timers[:i], c.timers[i+1:]...)
			return nil
		}
	}
	return &webcontrol.RejectedError{Op: "timerdelete", Text: "No matching Timer found"}
}
