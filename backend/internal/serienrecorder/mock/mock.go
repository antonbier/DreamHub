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
type Client struct{}

var _ serienrecorder.Client = (*Client)(nil)

// New returns a mock with fixture markers and upcoming episodes.
func New() *Client { return &Client{} }

func (*Client) Info(ctx context.Context) (*serienrecorder.Info, error) {
	return &serienrecorder.Info{APIVersion: "2.12", SRVersion: "4.8.0-beta"}, nil
}

func (*Client) Markers(ctx context.Context) ([]serienrecorder.Marker, error) {
	return []serienrecorder.Marker{
		{ID: 9, Name: "And Just Like That...", FSID: "and-just-like-that", WLID: "42185", Active: true, Info: "USA 2021-2025", Channels: "Alle", NumberOfRecords: 1},
		{ID: 10, Name: "Die Verräter - Vertraue Niemandem!", FSID: "die-verraeter-vertraue-niemandem", WLID: "47241", Active: true, Info: "D 2023-", Channels: "Alle", NumberOfRecords: 1},
	}, nil
}

func (*Client) UpcomingEpisodes(ctx context.Context) ([]serienrecorder.UpcomingEpisode, error) {
	return []serienrecorder.UpcomingEpisode{
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
	}, nil
}
