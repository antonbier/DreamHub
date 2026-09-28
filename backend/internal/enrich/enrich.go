// Package enrich adds what the box doesn't know about a title — poster,
// rating, overview, genres — from an external database, TMDB first
// (ROADMAP.md, GuideHub: "Online enrichment (TMDB)"). A core service, used
// by every hub that shows titles (guide, recordings, series,
// recommendations), reached only through the TMDB connector's own client
// and key.
//
// Three rules shape it:
//
//   - A wrong picture is worse than none. A result only counts when its
//     title (or original title) is the looked-up title, after
//     normalization (internal/titles) — never "the first search hit".
//   - No live dependency (CLAUDE.md): results, misses included, are cached
//     in SQLite, images on disk, so lists render from the cache and keep
//     their pictures while the internet or TMDB is down.
//   - The browser never talks to TMDB (PRINCIPLES.md: no external requests
//     from the UI): images are fetched and served by DreamHub itself.
//
// Sources, in order: what the box already keeps first — the optional
// DreamMediaInfo plugin's database (internal/dmi: overview, genres, rating,
// age rating and the exact TMDB ID, read-only) — then TMDB. When
// DreamMediaInfo knows a title, TMDB is only asked for its poster, by ID,
// so nothing is guessed from a search. Info.Source says where a result
// came from; more databases can join the same way.
package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dmi"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/titles"
	"github.com/antonbier/DreamHub/backend/internal/tmdb"
)

// Info is what enrichment knows about one title.
type Info struct {
	Source        string   `json:"source"` // "tmdb"
	Kind          string   `json:"kind"`   // "movie" or "tv"
	ID            int64    `json:"id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"originalTitle,omitempty"`
	Year          int      `json:"year,omitempty"`
	Overview      string   `json:"overview,omitempty"`
	Rating        float64  `json:"rating,omitempty"` // 0-10
	Votes         int      `json:"votes,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	FSK           string   `json:"fsk,omitempty"`      // German age rating (DreamMediaInfo)
	Poster        string   `json:"poster,omitempty"`   // image path, served by /api/v1/enrich/image
	Backdrop      string   `json:"backdrop,omitempty"` // image path
}

// Query is one title to look up. Kind ("movie"/"tv") is a hint that wins
// a tie; empty means either.
type Query struct {
	Title string `json:"title"`
	Kind  string `json:"kind,omitempty"`
}

// MaxBatch caps one Lookup call.
const MaxBatch = 40

const (
	hitTTL  = 30 * 24 * time.Hour // details barely change
	missTTL = 3 * 24 * time.Hour  // a title TMDB didn't know may appear later
	// concurrency and batchTimeout keep a list of uncached titles from
	// firing 40 requests at once or hanging a page: whatever isn't found
	// in time comes back empty now and is looked up again next time.
	concurrency  = 4
	batchTimeout = 8 * time.Second
	genreTTL     = 24 * time.Hour
)

// Service looks titles up and caches the answers.
type Service struct {
	// DMI is the optional DreamMediaInfo plugin's database; nil or absent
	// just skips it.
	DMI    *dmi.Reader
	TMDB   tmdb.Client
	Store  *store.Store
	Images *ImageCache
	Now    func() time.Time

	mu     sync.Mutex
	genres map[string]genreList // by language
}

type genreList struct {
	names   map[int]string
	fetched time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Lookup answers every query, in order: an Info, or nil when nothing
// matched, the answer isn't known yet, or apiKey is empty (then only
// cached answers are used).
func (s *Service) Lookup(ctx context.Context, apiKey, language string, qs []Query) []*Info {
	if len(qs) > MaxBatch {
		qs = qs[:MaxBatch]
	}
	for i := range qs {
		// A recording without a .ts.meta title shows its file name.
		qs[i].Title = titles.FromFileName(qs[i].Title)
	}
	out := make([]*Info, len(qs))
	var todo []int
	fromDMI := map[int]bool{}
	for i, q := range qs {
		if s.DMI != nil {
			if e, ok := s.DMI.Lookup(ctx, q.Title, q.Kind); ok {
				out[i] = fromDMIEntry(e)
				fromDMI[i] = true
				if apiKey != "" && s.TMDB != nil {
					todo = append(todo, i) // for the poster, by ID
				}
				continue
			}
		}
		info, fresh, ok := s.cached(ctx, language, q)
		if ok {
			out[i] = info
		}
		if (!ok || !fresh) && apiKey != "" && s.TMDB != nil {
			todo = append(todo, i)
		}
	}
	if len(todo) == 0 {
		return out
	}

	ctx, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, i := range todo {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if fromDMI[i] {
				mu.Lock()
				base := out[i]
				mu.Unlock()
				if pics, err := s.byID(ctx, apiKey, language, base.Kind, base.ID); err == nil {
					mu.Lock()
					out[i].Poster, out[i].Backdrop = pics.Poster, pics.Backdrop
					if out[i].Votes == 0 {
						out[i].Votes = pics.Votes
					}
					mu.Unlock()
				}
				return
			}
			info, err := s.fetch(ctx, apiKey, language, qs[i])
			if err != nil {
				return // not cached: tried again next time
			}
			mu.Lock()
			out[i] = info
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	return out
}

func cacheKey(language string, q Query) string {
	return "tmdb|" + language + "|" + q.Kind + "|" + titles.Normalize(q.Title)
}

// cached returns the cached answer for q, whether it's still fresh, and
// whether there is one at all.
func (s *Service) cached(ctx context.Context, language string, q Query) (*Info, bool, bool) {
	raw, at, ok, err := s.Store.Enrichment(ctx, cacheKey(language, q))
	if err != nil || !ok {
		return nil, false, false
	}
	age := s.now().Sub(time.Unix(at, 0))
	if raw == "" {
		return nil, age < missTTL, true
	}
	var info Info
	if json.Unmarshal([]byte(raw), &info) != nil {
		return nil, false, false
	}
	return &info, age < hitTTL, true
}

// fetch asks TMDB and caches the answer, a miss included. An error (TMDB
// unreachable, key rejected) is returned uncached.
func (s *Service) fetch(ctx context.Context, apiKey, language string, q Query) (*Info, error) {
	info, err := s.search(ctx, apiKey, language, q)
	if err != nil {
		return nil, err
	}
	raw := ""
	if info != nil {
		b, _ := json.Marshal(info)
		raw = string(b)
	}
	_ = s.Store.PutEnrichment(context.WithoutCancel(ctx), cacheKey(language, q), raw, s.now().Unix())
	return info, nil
}

func (s *Service) search(ctx context.Context, apiKey, language string, q Query) (*Info, error) {
	want := titles.Normalize(q.Title)
	if want == "" {
		return nil, nil
	}
	candidates := []string{q.Title}
	// An episode's broadcast title ("Tatort: Borowski …") is looked up as
	// its series when the full title finds nothing.
	if part := titles.SeriesPart(q.Title); part != q.Title {
		candidates = append(candidates, part)
	}
	for n, title := range candidates {
		results, err := s.TMDB.Search(ctx, apiKey, title, language)
		if err != nil {
			return nil, err
		}
		kind := q.Kind
		if n > 0 {
			kind = "tv" // the series part of an episode title
		}
		if best := pick(results, titles.Normalize(title), kind); best != nil {
			return s.toInfo(ctx, apiKey, language, *best), nil
		}
	}
	return nil, nil
}

// pick chooses the best exact-title match: the hinted kind first, then
// the one more people rated (the well-known film over an obscure namesake).
func pick(results []tmdb.Result, want, kind string) *tmdb.Result {
	var best *tmdb.Result
	score := func(r *tmdb.Result) int {
		s := r.Votes
		if kind != "" && r.Kind == kind {
			s += 1 << 30
		}
		return s
	}
	for i := range results {
		r := &results[i]
		if titles.Normalize(r.Title) != want && titles.Normalize(r.OriginalTitle) != want {
			continue
		}
		if best == nil || score(r) > score(best) {
			best = r
		}
	}
	return best
}

func (s *Service) toInfo(ctx context.Context, apiKey, language string, r tmdb.Result) *Info {
	info := &Info{
		Source: "tmdb", Kind: r.Kind, ID: r.ID, Title: r.Title, Year: r.Year, Overview: r.Overview,
		Rating: r.Rating, Votes: r.Votes, Poster: r.PosterPath, Backdrop: r.BackdropPath,
	}
	if r.OriginalTitle != r.Title {
		info.OriginalTitle = r.OriginalTitle
	}
	if len(r.GenreIDs) > 0 {
		names := s.genreNames(ctx, apiKey, language)
		for _, id := range r.GenreIDs {
			if n, ok := names[id]; ok {
				info.Genres = append(info.Genres, n)
			}
		}
	}
	return info
}

// genreNames is TMDB's genre list for language, kept in memory for a day;
// on failure, genres are just left out.
func (s *Service) genreNames(ctx context.Context, apiKey, language string) map[int]string {
	s.mu.Lock()
	g, ok := s.genres[language]
	s.mu.Unlock()
	if ok && s.now().Sub(g.fetched) < genreTTL {
		return g.names
	}
	names, err := s.TMDB.Genres(ctx, apiKey, language)
	if err != nil {
		return g.names
	}
	s.mu.Lock()
	if s.genres == nil {
		s.genres = map[string]genreList{}
	}
	s.genres[language] = genreList{names: names, fetched: s.now()}
	s.mu.Unlock()
	return names
}

// fromDMIEntry turns DreamMediaInfo's entry into an Info.
func fromDMIEntry(e dmi.Entry) *Info {
	return &Info{
		Source: "dreammediainfo", Kind: e.Kind, ID: e.TMDBID, Title: e.Title, Year: e.Year,
		Overview: e.Overview, Rating: e.Rating, Genres: e.Genres, FSK: e.FSK,
	}
}

// byID is TMDB's own record for an exact ID, cached like search answers.
func (s *Service) byID(ctx context.Context, apiKey, language, kind string, id int64) (*Info, error) {
	key := "tmdb|id|" + language + "|" + kind + "|" + strconv.FormatInt(id, 10)
	if raw, at, ok, err := s.Store.Enrichment(ctx, key); err == nil && ok && raw != "" && s.now().Sub(time.Unix(at, 0)) < hitTTL {
		var info Info
		if json.Unmarshal([]byte(raw), &info) == nil {
			return &info, nil
		}
	}
	r, err := s.TMDB.Details(ctx, apiKey, kind, id, language)
	if err != nil {
		return nil, err
	}
	info := s.toInfo(ctx, apiKey, language, r)
	if b, err := json.Marshal(info); err == nil {
		_ = s.Store.PutEnrichment(context.WithoutCancel(ctx), key, string(b), s.now().Unix())
	}
	return info, nil
}

// ErrNoImages is returned by Image when no image cache is configured.
var ErrNoImages = errors.New("enrich: no image cache")

// Image returns one image, from the disk cache or fetched from TMDB and
// cached. size and path are validated by the TMDB client.
func (s *Service) Image(ctx context.Context, size, path string) ([]byte, string, error) {
	if s.Images == nil || s.TMDB == nil {
		return nil, "", ErrNoImages
	}
	if !tmdb.ValidImage(size, path) {
		return nil, "", tmdb.ErrInvalidImage
	}
	if b, ct, ok := s.Images.Get(size, path); ok {
		return b, ct, nil
	}
	b, ct, err := s.TMDB.Image(ctx, size, path)
	if err != nil {
		return nil, "", err
	}
	s.Images.Put(size, path, b)
	return b, ct, nil
}

// Language maps DreamHub's UI language to TMDB's language parameter, so
// titles, overviews and genres come back in the language the user reads.
func Language(uiLanguage string) string {
	switch uiLanguage {
	case "de":
		return "de-DE"
	case "it":
		return "it-IT"
	}
	return "en-US"
}
