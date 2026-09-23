// Package mock implements webcontrol.Client against fixture data instead of
// a real device, so the rest of DreamHub is buildable and testable without
// a reachable Dreambox. See PRINCIPLES.md, "Mock-first development", and
// ROADMAP.md, Foundation.
//
// Fixture values below are taken directly from the live responses recorded
// in ARCHITECTURE.md, not invented, so behavior against the mock matches
// what was actually observed on the reference Dreambox One.
package mock

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Client is a webcontrol.Client backed by fixed, in-memory fixture data.
type Client struct{}

var _ webcontrol.Client = Client{}

func (Client) DeviceInfo(ctx context.Context) (*webcontrol.DeviceInfo, error) {
	return &webcontrol.DeviceInfo{
		EnigmaVersion: "5.0.0r117-gutemine-2024-08-12",
		ImageVersion:  "Experimental 2023-04-24",
		WebifVersion:  "1.9.1",
		DeviceName:    "one",
	}, nil
}

func (Client) MovieList(ctx context.Context) ([]webcontrol.Movie, error) {
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
	}, nil
}
