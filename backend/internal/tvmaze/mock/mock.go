// Package mock implements tvmaze.Client without network access, with
// episode lists matching the mock box's series.
package mock

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/antonbier/DreamHub/backend/internal/tvmaze"
)

// Client knows "Die Verräter" and "Tatort".
type Client struct {
	// Calls counts requests, for tests on caching.
	Calls atomic.Int64
}

var _ tvmaze.Client = (*Client)(nil)

func New() *Client { return &Client{} }

func (c *Client) SearchShows(ctx context.Context, name string) ([]tvmaze.Show, error) {
	c.Calls.Add(1)
	q := strings.ToLower(name)
	var out []tvmaze.Show
	if strings.Contains(q, "verräter") || strings.Contains(q, "verrater") {
		out = append(out, tvmaze.Show{ID: 501, Name: "Die Verräter - Vertraue Niemandem!", Premiered: "2023-10-05", Language: "German", Score: 0.9})
	}
	if strings.Contains(q, "gute zeiten") {
		out = append(out, tvmaze.Show{ID: 503, Name: "Gute Zeiten, schlechte Zeiten", Premiered: "1992-05-11", Language: "German", Score: 0.9})
	}
	if q == "fbi" {
		out = append(out, tvmaze.Show{ID: 601, Name: "FBI", Premiered: "2018-09-25", Language: "English", Score: 0.9},
			tvmaze.Show{ID: 602, Name: "The FBI", Premiered: "1965-09-19", Language: "English", Score: 0.5})
	}
	if strings.Contains(q, "tatort") {
		out = append(out, tvmaze.Show{ID: 777, Name: "Tatort-Reiniger", Premiered: "2011-12-18", Language: "German", Score: 0.6},
			tvmaze.Show{ID: 502, Name: "Tatort", Premiered: "1970-11-29", Language: "German", Score: 0.8})
	}
	return out, nil
}

func (c *Client) Episodes(ctx context.Context, showID int64) ([]tvmaze.Episode, error) {
	c.Calls.Add(1)
	switch showID {
	case 501:
		return []tvmaze.Episode{
			{Season: 1, Number: 1, Name: "Die Burg", Airdate: "2023-10-05", Airstamp: 1696532400},
			{Season: 1, Number: 2, Name: "Der erste Verrat", Airdate: "2023-10-12", Airstamp: 1697137200},
			{Season: 1, Number: 3, Name: "Misstrauen", Airdate: "2023-10-19", Airstamp: 1697742000},
			{Season: 2, Number: 1, Name: "Neue Runde", Airdate: "2024-10-03", Airstamp: 1727982000},
			{Season: 2, Number: 2, Name: "Das Finale", Airdate: "2030-01-01", Airstamp: 1893484800},
		}, nil
	case 503:
		// TVmaze files the daily soap under years but keeps counting
		// episodes — the numbers SerienRecorder uses (as on the box).
		return []tvmaze.Episode{
			{Season: 1992, Number: 1, Name: "Folge 1", Airdate: "1992-05-11", Airstamp: 705610800},
			{Season: 2026, Number: 8613, Name: "(K)Ein Abschied", Airdate: "2026-09-21", Airstamp: 1790011920},
			{Season: 2026, Number: 8614, Name: "Hirngespinst", Airdate: "2026-09-22", Airstamp: 1790098320},
			{Season: 2026, Number: 8615, Name: "Fehler der Vergangenheit", Airdate: "2026-09-25", Airstamp: 1790319120},
		}, nil
	case 502:
		return []tvmaze.Episode{
			{Season: 2026, Number: 30, Name: "Borowski und das Haus am Meer", Airdate: "2026-09-20", Airstamp: 1790021700},
			{Season: 2026, Number: 31, Name: "Borowski und der gute Mensch", Airdate: "2026-09-28", Airstamp: 1790626500},
		}, nil
	}
	return nil, nil
}
