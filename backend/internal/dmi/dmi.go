// Package dmi reads what the optional DreamMediaInfo plugin already keeps
// about titles — overview, genres, cast, rating, German age rating and the
// TMDB ID — so DreamHub doesn't look up and store the same data a second
// time (the user's wish: data kept once, by whoever already keeps it).
// Read-only: DreamHub never writes DreamMediaInfo's database, and without
// the plugin (no database file) every lookup just finds nothing.
//
// Its database (ARCHITECTURE.md, "DreamMediaInfo"): SQLite at the path in
// /etc/enigma2/settings (config.plugins.dreammediainfo.db_path, on the
// reference box /data/dreammediainfo.db), table media_cache with key =
// lower-cased title + "_movie" / "_tv" (films also "_<year>_movie"), data =
// TMDB's details as JSON.
package dmi

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DefaultPath is where DreamMediaInfo keeps its database when its settings
// don't say otherwise (the reference box's value).
const DefaultPath = "/data/dreammediainfo.db"

// Entry is one title as DreamMediaInfo knows it.
type Entry struct {
	TMDBID   int64
	Kind     string // "movie" or "tv"
	Title    string
	Year     int
	Overview string
	Genres   []string
	Rating   float64 // TMDB's vote average, 0-10; 0 = none
	FSK      string  // German age rating ("12"), "" when unknown
}

// Reader looks titles up in DreamMediaInfo's database.
type Reader struct {
	// Path of the database; "" means: from /etc/enigma2/settings, else
	// DefaultPath.
	Path string
}

// SettingsFile is Enigma2's settings file, where DreamMediaInfo's
// database path is configured.
var SettingsFile = "/etc/enigma2/settings"

func (r *Reader) path() string {
	if r.Path != "" {
		return r.Path
	}
	if f, err := os.Open(SettingsFile); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "config.plugins.dreammediainfo.db_path="); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return DefaultPath
}

// Available reports whether DreamMediaInfo's database exists here.
func (r *Reader) Available() bool {
	fi, err := os.Stat(r.path())
	return err == nil && fi.Mode().IsRegular()
}

// Lookup finds a title; kind "movie", "tv" or "" (either). ok=false when
// the plugin isn't there or doesn't know the title.
func (r *Reader) Lookup(ctx context.Context, title, kind string) (Entry, bool) {
	p := r.path()
	if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
		return Entry{}, false
	}
	db, err := open(p)
	if err != nil {
		return Entry{}, false
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	lower := strings.ToLower(strings.TrimSpace(title))
	// DreamMediaInfo writes a colon as " -" in its keys ("fbi - special
	// crime unit_tv", seen on the reference box): try both spellings.
	bases := []string{lower}
	if alt := strings.ReplaceAll(lower, ":", " -"); alt != lower {
		bases = append(bases, alt)
	}
	var queries []struct {
		sql  string
		args []any
	}
	add := func(q string, args ...any) {
		queries = append(queries, struct {
			sql  string
			args []any
		}{q, args})
	}
	const one = "SELECT type, data FROM media_cache WHERE key = ?"
	for _, base := range bases {
		likeBase := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(base)
		if kind != "movie" {
			add(one, base+"_tv")
		}
		if kind != "tv" {
			add(one, base+"_movie")
			add("SELECT type, data FROM media_cache WHERE key LIKE ? ESCAPE '\\' AND type = 'movie' ORDER BY ts DESC LIMIT 1", likeBase+"\\_%\\_movie")
		}
	}
	for _, q := range queries {
		var typ, data string
		if err := db.QueryRowContext(ctx, q.sql, q.args...).Scan(&typ, &data); err != nil {
			continue
		}
		if e, ok := parse(typ, data); ok {
			return e, true
		}
	}
	return Entry{}, false
}

// open opens the database read-only, never blocking the plugin for long:
// it writes to this database itself.
func open(p string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+p+"?mode=ro&_pragma=busy_timeout(1500)")
}

// EpisodeCode is which episode an EPG broadcast is, from DreamMediaInfo's
// match_cache (key "<tmdb id>|<episode title>", value "S05E14": the EPG
// episode titles it has already matched against TMDB's episode lists).
// episodeTitles are the candidates to try in order (the EPG's short
// description, the part of the title after the series name); the
// comparison ignores case. ok=false when nothing matched.
func (r *Reader) EpisodeCode(ctx context.Context, tmdbID int64, episodeTitles ...string) (season, episode int, ok bool) {
	p := r.path()
	if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() || tmdbID == 0 {
		return 0, 0, false
	}
	db, err := open(p)
	if err != nil {
		return 0, 0, false
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, t := range episodeTitles {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		var se string
		key := strconv.FormatInt(tmdbID, 10) + "|" + t
		if db.QueryRowContext(ctx, "SELECT se FROM match_cache WHERE key = ? COLLATE NOCASE LIMIT 1", key).Scan(&se) != nil {
			continue
		}
		if m := seCode.FindStringSubmatch(se); m != nil {
			season, _ = strconv.Atoi(m[1])
			episode, _ = strconv.Atoi(m[2])
			if episode > 0 {
				return season, episode, true
			}
		}
	}
	return 0, 0, false
}

var seCode = regexp.MustCompile(`(?i)^\s*S(\d{1,4})\s*E(\d{1,4})\s*$`)

func parse(typ, data string) (Entry, bool) {
	var d struct {
		ID           int64   `json:"id"`
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		Overview     string  `json:"overview"`
		ReleaseDate  string  `json:"release_date"`
		FirstAirDate string  `json:"first_air_date"`
		VoteAverage  float64 `json:"vote_average"`
		FSK          string  `json:"fsk"`
		Genres       []struct {
			Name string `json:"name"`
		} `json:"genres"`
	}
	if json.Unmarshal([]byte(data), &d) != nil || d.ID == 0 {
		return Entry{}, false
	}
	e := Entry{TMDBID: d.ID, Kind: typ, Title: d.Title, Overview: strings.TrimSpace(d.Overview), Rating: d.VoteAverage, FSK: d.FSK}
	date := d.ReleaseDate
	if typ == "tv" {
		e.Title, date = d.Name, d.FirstAirDate
	}
	if len(date) >= 4 {
		for _, c := range date[:4] {
			e.Year = e.Year*10 + int(c-'0')
		}
		if e.Year < 1800 {
			e.Year = 0
		}
	}
	for _, g := range d.Genres {
		e.Genres = append(e.Genres, g.Name)
	}
	return e, e.Title != ""
}
