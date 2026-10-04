// Package mock implements omdb.Client without network access or a key.
package mock

import (
	"context"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/omdb"
)

type Client struct{}

func New() *Client { return &Client{} }

var _ omdb.Client = (*Client)(nil)

// Ratings makes up stable ratings from the ID's digits.
func (*Client) Ratings(ctx context.Context, apiKey, imdbID string) (omdb.Ratings, error) {
	if !omdb.ValidIMDbID(imdbID) {
		return omdb.Ratings{}, omdb.ErrInvalidID
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(imdbID, "tt"))
	return omdb.Ratings{IMDb: float64(50+n%45) / 10, IMDbVotes: 1000 + n, RottenTomatoes: 40 + n%60, Metascore: 30 + n%65}, nil
}
