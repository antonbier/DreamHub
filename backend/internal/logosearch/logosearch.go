// Package logosearch finds a TV channel's logo on the internet, for the
// channels the box's own picon sets don't cover. The source is the
// public tv-logo/tv-logos collection on GitHub (one PNG per channel, by
// country); this package reads its file list once a day, matches a
// channel's name against it, and fetches the one file the person picks.
// A channel the collection doesn't have simply gets no candidates — the
// person can still upload their own picture.
//
// Nothing here is called unless someone asks for a logo in ChannelHub;
// DreamHub never fetches logos by itself (PRINCIPLES.md: no hidden
// network use).
package logosearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry is one logo of the collection.
type Entry struct {
	// Path is the file's path in the collection ("countries/austria/atv-at.png").
	Path string `json:"path"`
	// Name is a readable name derived from the file name ("atv").
	Name string `json:"name"`
	// Country is the folder it lies in ("austria").
	Country string `json:"country"`
	norm    string
}

// Source finds and fetches logos. HTTP is the real one; ./mock a demo.
type Source interface {
	// Search returns the best matches for a channel name, best first.
	Search(ctx context.Context, name string, limit int) ([]Entry, error)
	// Fetch downloads one Entry.Path's picture.
	Fetch(ctx context.Context, path string) ([]byte, error)
}

// ErrUnavailable: the collection can't be reached right now.
var ErrUnavailable = errors.New("logosearch: the logo collection isn't reachable")

// ErrInvalidPath: not a PNG inside the collection's countries folder.
var ErrInvalidPath = errors.New("logosearch: not a logo of the collection")

// maxLogoBytes bounds one downloaded picture.
const maxLogoBytes = 2 << 20

// ValidPath: exactly a countries/<country>/…/<file>.png path — what the
// file list contains, with no way to point anywhere else on the host.
func ValidPath(p string) bool {
	if !strings.HasPrefix(p, "countries/") || !strings.HasSuffix(p, ".png") || strings.Contains(p, "..") || strings.ContainsAny(p, "?#\\ \t\r\n") {
		return false
	}
	return path.Clean(p) == p
}

// HTTP reads the collection from GitHub.
type HTTP struct {
	// TreeURL and RawBase are overridden in tests.
	TreeURL string
	RawBase string
	Client  *http.Client

	mu      sync.Mutex
	entries []Entry
	loaded  time.Time
}

const (
	defaultTreeURL = "https://api.github.com/repos/tv-logo/tv-logos/git/trees/main?recursive=1"
	defaultRawBase = "https://raw.githubusercontent.com/tv-logo/tv-logos/main/"
	indexTTL       = 24 * time.Hour
)

var _ Source = (*HTTP)(nil)

func (h *HTTP) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (h *HTTP) index(ctx context.Context) ([]Entry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.entries != nil && time.Since(h.loaded) < indexTTL {
		return h.entries, nil
	}
	url := h.TreeURL
	if url == "" {
		url = defaultTreeURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := h.client().Do(req)
	if err != nil {
		return h.stale(ErrUnavailable)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return h.stale(ErrUnavailable)
	}
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&tree); err != nil {
		return h.stale(ErrUnavailable)
	}
	var out []Entry
	for _, t := range tree.Tree {
		if t.Type != "blob" || !ValidPath(t.Path) {
			continue
		}
		parts := strings.Split(t.Path, "/")
		if len(parts) < 3 {
			continue
		}
		name := nameOf(parts[len(parts)-1])
		out = append(out, Entry{Path: t.Path, Name: name, Country: parts[1], norm: Normalize(name)})
	}
	if len(out) == 0 {
		return h.stale(ErrUnavailable)
	}
	h.entries, h.loaded = out, time.Now()
	return out, nil
}

// stale keeps serving an older list when a refresh fails.
func (h *HTTP) stale(err error) ([]Entry, error) {
	if h.entries != nil {
		return h.entries, nil
	}
	return nil, err
}

func (h *HTTP) Search(ctx context.Context, name string, limit int) ([]Entry, error) {
	entries, err := h.index(ctx)
	if err != nil {
		return nil, err
	}
	return Rank(entries, name, limit), nil
}

func (h *HTTP) Fetch(ctx context.Context, p string) ([]byte, error) {
	if !ValidPath(p) {
		return nil, ErrInvalidPath
	}
	base := h.RawBase
	if base == "" {
		base = defaultRawBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/"+p, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client().Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxLogoBytes+1))
	if err != nil || len(b) > maxLogoBytes {
		return nil, fmt.Errorf("logosearch: picture too large or unreadable")
	}
	return b, nil
}

// nameOf is a file's readable name: "atv-hd-at.png" → "atv-hd-at".
func nameOf(file string) string { return strings.TrimSuffix(file, ".png") }

var (
	nonAlnum  = regexp.MustCompile(`[^a-z0-9]+`)
	genericWs = map[string]bool{"hd": true, "uhd": true, "sd": true, "tv": true, "television": true, "fernsehen": true, "channel": true, "kanal": true}
	countryWs = map[string]bool{"at": true, "de": true, "ch": true, "it": true, "fr": true, "es": true, "uk": true, "us": true, "nl": true, "pl": true, "tr": true}
)

var accents = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "é", "e", "è", "e", "ê", "e", "à", "a", "á", "a", "â", "a", "ó", "o", "ò", "o", "ô", "o", "í", "i", "ì", "i", "ú", "u", "ù", "u", "ñ", "n", "ç", "c")

// words splits a channel or file name into lower-case words without
// accents, generic ones (HD, TV) and the country suffix of a file name
// dropped.
func words(s string) []string {
	s = accents.Replace(strings.ToLower(s))
	var out []string
	for _, w := range nonAlnum.Split(s, -1) {
		if w == "" || genericWs[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// Normalize is a name squashed to compare "ORF 1 HD", "orf1" and
// "orf-1-hd-at" as the same thing: the words joined without separators.
func Normalize(s string) string {
	ws := words(s)
	// A trailing two-letter country code of a file name carries no
	// information about the channel.
	if len(ws) > 1 && countryWs[ws[len(ws)-1]] {
		ws = ws[:len(ws)-1]
	}
	return strings.Join(ws, "")
}

// Rank returns up to limit entries matching name, best first: an equal
// squashed name first, then one that starts with the other, then one that
// contains it (names shorter than 3 characters only match exactly).
func Rank(entries []Entry, name string, limit int) []Entry {
	want := Normalize(name)
	if want == "" || limit <= 0 {
		return []Entry{}
	}
	type scored struct {
		e     Entry
		score int
	}
	var hits []scored
	for _, e := range entries {
		n := e.norm
		if n == "" {
			n = Normalize(e.Name)
		}
		score := 0
		switch {
		case n == want:
			score = 100
		case len(want) >= 3 && strings.HasPrefix(n, want):
			score = 70 - min(len(n)-len(want), 30)
		case len(n) >= 3 && strings.HasPrefix(want, n):
			score = 60 - min(len(want)-len(n), 30)
		case len(want) >= 4 && strings.Contains(n, want):
			score = 40 - min(len(n)-len(want), 30)
		}
		if score > 0 {
			// A plain file beats its HD variant of the same channel.
			if strings.Contains(e.Path, "/hd/") {
				score--
			}
			hits = append(hits, scored{e, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].e.Path < hits[j].e.Path
	})
	out := make([]Entry, 0, min(limit, len(hits)))
	for i := 0; i < len(hits) && i < limit; i++ {
		out = append(out, hits[i].e)
	}
	return out
}
