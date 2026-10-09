// Package mock implements serienrecorder.Client against fixture data
// captured live 2026-09-24 (ARCHITECTURE.md, issue #10), so the rest of
// DreamHub is buildable and testable without a reachable Dreambox — see
// PRINCIPLES.md, "Mock-first development".
package mock

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
)

// Client is a serienrecorder.Client backed by in-memory fixture data.
type Client struct {
	// Shift moves the upcoming episodes' times (seconds), along with the
	// webcontrol mock's fixture day (DREAMHUB_MOCK_DATE).
	Shift int64
}

var _ serienrecorder.Client = (*Client)(nil)

// New returns a mock with fixture markers and upcoming episodes.
func New() *Client { return &Client{} }

func (*Client) Info(ctx context.Context) (*serienrecorder.Info, error) {
	return &serienrecorder.Info{APIVersion: "2.12", SRVersion: "4.8.0-beta"}, nil
}

func (*Client) Markers(ctx context.Context) ([]serienrecorder.Marker, error) {
	return []serienrecorder.Marker{
		// A daily soap: SerienRecorder numbers it absolutely (S00E8615),
		// as on the reference box.
		{ID: 8, Name: "Gute Zeiten, schlechte Zeiten", FSID: "gute-zeiten-schlechte-zeiten", WLID: "5164", Active: true, Info: "D 1992-", Channels: "Alle", NumberOfRecords: 1, Seasons: "Alle (inkl. Specials)", RecordFolder: "/media/hdd/movie/"},
		{ID: 9, Name: "And Just Like That...", FSID: "and-just-like-that", WLID: "42185", Active: true, Info: "USA 2021-2025", Channels: "Alle", NumberOfRecords: 1, Seasons: "Alle (inkl. Specials)", RecordFolder: "/media/hdd/movie/"},
		{ID: 10, Name: "Die Verräter - Vertraue Niemandem!", FSID: "die-verraeter-vertraue-niemandem", WLID: "47241", Active: true, Info: "D 2023-", Channels: "Alle", NumberOfRecords: 1, Seasons: "Alle (inkl. Specials)", RecordFolder: "/media/hdd/movie/"},
		// Matches the mock box's "Tatort/" recording folder and its EPG,
		// so SeriesHub's detail view has something on every source.
		{ID: 11, Name: "Tatort", FSID: "tatort", WLID: "1234", Active: true, Info: "D 1970-", Channels: "Das Erste HD", NumberOfRecords: 1, Seasons: "Alle", RecordFolder: "/media/hdd/movie/"},
	}, nil
}

func (c *Client) UpcomingEpisodes(ctx context.Context) ([]serienrecorder.UpcomingEpisode, error) {
	list := []serienrecorder.UpcomingEpisode{
		{
			Series: "Gute Zeiten, schlechte Zeiten", Title: "Fehler der Vergangenheit",
			Channel: "RTL Television", ServiceReference: "1:0:1:2EE3:441:1:C00000:0:0:0:",
			Season: "0", Episode: "8615", StartTime: 1790319120, FSID: "gute-zeiten-schlechte-zeiten",
		},
		{
			Series: "Gute Zeiten, schlechte Zeiten", Title: "Echte Reue?",
			Channel: "RTL Television", ServiceReference: "1:0:1:2EE3:441:1:C00000:0:0:0:",
			Season: "0", Episode: "8616", StartTime: 1790357520, FSID: "gute-zeiten-schlechte-zeiten",
		},
		{
			Series: "Tatort", Title: "Borowski und der gute Mensch",
			Channel: "Das Erste HD", ServiceReference: "1:0:19:283D:3FB:1:C00000:0:0:0:",
			Season: "2026", Episode: "31", StartTime: 1790626500, FSID: "tatort",
		},
	}
	for k := range list {
		list[k].StartTime += c.Shift
	}
	return list, nil
}

func (*Client) SeriesInfo(ctx context.Context, wlid, fsid string) (*serienrecorder.SeriesInfo, error) {
	if fsid == "tatort" {
		return &serienrecorder.SeriesInfo{Title: "Tatort", Year: "1970-", Info: "Krimi (90 min)",
			Description: "Die Krimireihe der ARD mit wechselnden Ermittlerteams.", Cast: []string{"Axel Milberg (Klaus Borowski)"}}, nil
	}
	return nil, nil
}

// Absent is a box without SerienRecorder: every call fails the way the
// real client does when the plugin's API isn't there.
type Absent struct{}

var _ serienrecorder.Client = Absent{}

func (Absent) Info(context.Context) (*serienrecorder.Info, error) {
	return nil, serienrecorder.ErrNotInstalled
}
func (Absent) Markers(context.Context) ([]serienrecorder.Marker, error) {
	return nil, serienrecorder.ErrNotInstalled
}
func (Absent) UpcomingEpisodes(context.Context) ([]serienrecorder.UpcomingEpisode, error) {
	return nil, serienrecorder.ErrNotInstalled
}
func (Absent) SeriesInfo(context.Context, string, string) (*serienrecorder.SeriesInfo, error) {
	return nil, serienrecorder.ErrNotInstalled
}
