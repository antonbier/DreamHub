package hubs

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
)

// Guide sources and the coverage check (docs/NETCAST_PLAN.md, N5). A source
// is an XMLTV file: from the curated list, an own address, or a file found in
// a GitHub repository. The check reads only the file's channel list and says
// how many of the chosen channels of a read playlist the source knows. It
// never writes anything: adding a source to Netcast needs the box (N3).

//go:embed guide_sources.json
var curatedGuideJSON []byte

// GuideSource is an entry of the curated list.
type GuideSource struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Region string `json:"region,omitempty"`
}

func curatedGuideSources() []GuideSource {
	var l []GuideSource
	_ = json.Unmarshal(curatedGuideJSON, &l)
	return l
}

const (
	repoCacheTTL   = 30 * time.Minute // GitHub allows 60 unauthenticated calls an hour
	repoCacheLimit = 64
	guideCacheTTL  = 10 * time.Minute
	guideCacheMax  = 8
)

type repoCacheEntry struct {
	at      time.Time
	listing *iptv.RepoListing
}

type guideCacheEntry struct {
	at time.Time
	gc *iptv.GuideChannels
}

var (
	guideMu    sync.Mutex
	repoCache  = map[string]repoCacheEntry{}
	guideCache = map[string]guideCacheEntry{}
)

func (h *Channels) guideSources(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sources": curatedGuideSources()})
}

// githubFiles lists the guide and playlist files in a repository folder.
func (h *Channels) githubFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.URL) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"url"}`)
		return
	}
	repo, ok := iptv.ParseRepo(body.URL)
	if !ok {
		httpx.WriteError(w, r, http.StatusBadRequest, CodeIPTVBadURL, "that is not a GitHub repository address")
		return
	}
	key := repo.Owner + "/" + repo.Name + "@" + repo.Ref + ":" + repo.Dir
	now := h.now()
	guideMu.Lock()
	c, hit := repoCache[key]
	guideMu.Unlock()
	if !hit || now.Sub(c.at) > repoCacheTTL {
		l, err := h.fetcher().ListRepo(r.Context(), repo)
		if err != nil {
			writeFetchError(w, r, err)
			return
		}
		c = repoCacheEntry{at: now, listing: l}
		guideMu.Lock()
		if len(repoCache) >= repoCacheLimit {
			repoCache = map[string]repoCacheEntry{}
		}
		repoCache[key] = c
		guideMu.Unlock()
	}
	files := make([]map[string]any, 0, len(c.listing.Files))
	for _, f := range c.listing.Files {
		files = append(files, map[string]any{"name": f.Name, "path": f.Path, "kind": f.Kind, "size": f.Size, "url": f.URL})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"repo": repo.Owner + "/" + repo.Name, "ref": repo.Ref, "dir": repo.Dir, "files": files, "dirs": c.listing.Dirs,
	})
}

type guideMiss struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// guideCoverage reads a source's channel list and matches it against the
// chosen channels of a read playlist.
func (h *Channels) guideCoverage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		IDs   []int  `json:"ids"` // chosen channels; empty means the whole list
		URL   string `json:"url"` // the guide file
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.URL) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"token", "ids", "url"}`)
		return
	}
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	sess, ok := h.iptvSession(body.Token, user.ID)
	if !ok {
		httpx.WriteError(w, r, http.StatusGone, CodeIPTVSessionGone, "the list was read too long ago; read it again")
		return
	}
	ids := body.IDs
	if len(ids) == 0 {
		for i := range sess.pl.Entries {
			ids = append(ids, i)
		}
	}
	if len(ids) > iptvMaxPick {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "too many channels")
		return
	}
	for _, id := range ids {
		if id < 0 || id >= len(sess.pl.Entries) {
			httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "no such channel", map[string]httpx.FieldError{"ids": {Code: "invalid"}})
			return
		}
	}
	url := strings.TrimSpace(body.URL)
	now := h.now()
	guideMu.Lock()
	c, hit := guideCache[url]
	guideMu.Unlock()
	if !hit || now.Sub(c.at) > guideCacheTTL {
		gc, err := h.fetcher().ReadGuideChannels(ctx, url)
		if err != nil {
			writeGuideError(w, r, err)
			return
		}
		c = guideCacheEntry{at: now, gc: gc}
		guideMu.Lock()
		if len(guideCache) >= guideCacheMax {
			guideCache = map[string]guideCacheEntry{}
		}
		guideCache[url] = c
		guideMu.Unlock()
	}

	byID := map[string]bool{}
	byName := map[string]bool{}
	for _, gc := range c.gc.Channels {
		byID[strings.ToLower(gc.ID)] = true
		for _, n := range gc.Names {
			if k := normName(n); k != "" {
				byName[k] = true
			}
		}
		// ids such as "Das.Erste.de" carry the name too
		if k := normName(strings.TrimSuffix(gc.ID, guideIDSuffix(gc.ID))); k != "" {
			byName[k] = true
		}
	}
	known := 0
	var missing []guideMiss
	for _, id := range ids {
		e := sess.pl.Entries[id]
		hit := e.TvgID != "" && byID[strings.ToLower(strings.TrimSpace(e.TvgID))]
		if !hit {
			hit = byName[normName(e.Name)] || (e.TvgName != "" && byName[normName(e.TvgName)])
		}
		if hit {
			known++
		} else {
			missing = append(missing, guideMiss{ID: id, Name: e.Name})
		}
	}
	sort.SliceStable(missing, func(i, j int) bool { return strings.ToLower(missing[i].Name) < strings.ToLower(missing[j].Name) })
	shown := missing
	if len(shown) > 200 {
		shown = shown[:200]
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"known": known, "total": len(ids), "sourceChannels": len(c.gc.Channels), "truncated": c.gc.Truncated,
		"missing": shown, "missingCount": len(missing),
	})
}

// guideIDSuffix is the country part of an XMLTV id like "Das.Erste.de".
func guideIDSuffix(id string) string {
	if i := strings.LastIndex(id, "."); i > 0 && len(id)-i <= 4 {
		return id[i:]
	}
	return ""
}

func writeGuideError(w http.ResponseWriter, r *http.Request, err error) {
	if fe, ok := err.(*iptv.FetchError); ok {
		switch fe.Kind {
		case iptv.KindNotGuide:
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, CodeGuideNotXMLTV, "that is not an XMLTV guide file")
			return
		case iptv.KindUnsupported:
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, CodeGuideUnsupported, ".xz files cannot be read here; use the .xml or .xml.gz file")
			return
		}
	}
	writeFetchError(w, r, err)
}

// Error codes of the guide routes (the fetch errors are the IPTV ones).
const (
	CodeGuideNotXMLTV    = "guide_not_xmltv"
	CodeGuideUnsupported = "guide_unsupported_format"
)
