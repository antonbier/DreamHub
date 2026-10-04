package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/fanart"
	"github.com/antonbier/DreamHub/backend/internal/omdb"
)

// Extras is what further databases add to a title already identified by
// its TMDB ID — only for a detail view, never for a whole list, so the
// free tiers' daily limits (OMDb: 1,000) are never in reach:
//
//   - OMDb: Rotten Tomatoes' Tomatometer, IMDb's rating, Metacritic's
//     Metascore, by the IMDb ID TMDB supplies;
//   - fanart.tv: the title's logo, by TMDB's ID (films) or TheTVDB's
//     (series, also from TMDB).
//
// Each source is optional (no key: left out). Answers, misses included,
// are cached in the store for a week (a day for a miss), so a title is
// asked about at most once a week and still shows its extras offline.
type Extras struct {
	IMDbID         string  `json:"imdbId,omitempty"`
	IMDb           float64 `json:"imdb,omitempty"`
	IMDbVotes      int     `json:"imdbVotes,omitempty"`
	RottenTomatoes int     `json:"rottenTomatoes,omitempty"` // percent
	Metascore      int     `json:"metascore,omitempty"`
	Logo           string  `json:"logo,omitempty"` // fanart.tv image path, served by /api/v1/enrich/logo
}

// ExtrasKeys are the API keys the extras need; an empty one skips its
// source.
type ExtrasKeys struct {
	TMDB, OMDb, Fanart string
}

const (
	extrasTTL     = 7 * 24 * time.Hour
	extrasMissTTL = 24 * time.Hour
	extrasTimeout = 8 * time.Second
)

// Extras answers one title (kind "movie"/"tv", TMDB ID). nil when nothing
// is known and nothing could be asked.
func (s *Service) Extras(ctx context.Context, keys ExtrasKeys, language, kind string, id int64) *Extras {
	if (kind != "movie" && kind != "tv") || id <= 0 {
		return nil
	}
	// Which sources are on is part of the key: adding a key later asks
	// again instead of serving the older, poorer answer for a week.
	key := "extras|" + kind + "|" + strconv.FormatInt(id, 10) + "|" + on(keys.OMDb != "" && s.OMDb != nil) + on(keys.Fanart != "" && s.Fanart != nil)
	if raw, at, ok, err := s.Store.Enrichment(ctx, key); err == nil && ok {
		var x Extras
		ttl := extrasTTL
		if raw == "" || json.Unmarshal([]byte(raw), &x) != nil || x == (Extras{}) {
			ttl = extrasMissTTL
		}
		if s.now().Sub(time.Unix(at, 0)) < ttl {
			if x == (Extras{}) {
				return nil
			}
			return &x
		}
	}
	if keys.TMDB == "" || s.TMDB == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, extrasTimeout)
	defer cancel()
	imdbID, tvdbID, err := s.TMDB.ExternalIDs(ctx, keys.TMDB, kind, id)
	if err != nil {
		return nil // not cached: tried again next time
	}
	x := Extras{IMDbID: imdbID}
	failed := false
	if keys.OMDb != "" && s.OMDb != nil && omdb.ValidIMDbID(imdbID) {
		r, err := s.OMDb.Ratings(ctx, keys.OMDb, imdbID)
		if err != nil {
			failed = true
		}
		x.IMDb, x.IMDbVotes, x.RottenTomatoes, x.Metascore = r.IMDb, r.IMDbVotes, r.RottenTomatoes, r.Metascore
	}
	if keys.Fanart != "" && s.Fanart != nil {
		fid := id
		if kind == "tv" {
			fid = tvdbID
		}
		if fid > 0 {
			logo, err := s.Fanart.Logo(ctx, keys.Fanart, kind, fid, fanartLangs(language))
			if err != nil {
				failed = true
			}
			x.Logo = logo
		}
	}
	if !failed {
		b, _ := json.Marshal(x)
		_ = s.Store.PutEnrichment(context.WithoutCancel(ctx), key, string(b), s.now().Unix())
	}
	if x == (Extras{IMDbID: imdbID}) {
		return nil // nothing beyond the ID
	}
	return &x
}

func on(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// fanartLangs prefers the UI's language, then English.
func fanartLangs(language string) []string {
	lang := language
	if len(lang) >= 2 {
		lang = lang[:2]
	}
	if lang == "en" || lang == "" {
		return []string{"en"}
	}
	return []string{lang, "en"}
}

// ErrNoFanart is returned by Logo when fanart.tv isn't wired.
var ErrNoFanart = errors.New("enrich: no fanart client")

// Logo returns one fanart.tv picture, from the disk cache or fetched once.
func (s *Service) Logo(ctx context.Context, path string) ([]byte, string, error) {
	if s.Images == nil || s.Fanart == nil {
		return nil, "", ErrNoFanart
	}
	if !fanart.ValidImage(path) {
		return nil, "", fanart.ErrInvalidImage
	}
	if b, ct, ok := s.Images.Get("fanart", path); ok {
		return b, ct, nil
	}
	b, ct, err := s.Fanart.Image(ctx, path)
	if err != nil {
		return nil, "", err
	}
	s.Images.Put("fanart", path, b)
	return b, ct, nil
}
