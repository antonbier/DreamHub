// Package mock implements tmdb.Client without network access or a key.
package mock

import (
	"context"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/tmdb"
)

// Client knows a handful of titles; anything else comes back as a film of
// that name, so every flow has something to show.
type Client struct{}

var _ tmdb.Client = (*Client)(nil)

func New() *Client { return &Client{} }

func (*Client) Search(ctx context.Context, apiKey, query, language string) ([]tmdb.Result, error) {
	q := strings.ToLower(query)
	switch {
	case strings.Contains(q, "bauer sucht frau"):
		return []tmdb.Result{{Kind: "tv", ID: 1001, Title: "Bauer sucht Frau International", Year: 2012}}, nil
	case strings.Contains(q, "tatort"):
		return []tmdb.Result{{Kind: "tv", ID: 1002, Title: "Tatort", Year: 1970}, {Kind: "movie", ID: 2001, Title: "Tatort: Borowski und das Haus am Meer", Year: 2026}}, nil
	}
	return []tmdb.Result{{Kind: "movie", ID: 9999, Title: query, Year: 2020}}, nil
}

func (*Client) EpisodeTitle(ctx context.Context, apiKey string, tvID int64, season, episode int, language string) (string, error) {
	return "Mock-Episode", nil
}
