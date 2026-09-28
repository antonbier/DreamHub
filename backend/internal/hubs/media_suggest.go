package hubs

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/enrich"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/naming"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/tmdb"
)

// Name suggestions for a rename (decision D2). Three sources, all optional
// except the first, all ending in the same rename preview:
//
//   - "metadata": the recording's own .ts.meta and folder names — free,
//     offline, always there.
//   - "tmdb": a TMDB search on the title (and the episode's title for a
//     series), with the user's TMDB key.
//   - "ai": the AI provider reads the metadata and returns fields; counts
//     against ai.dailyLimit like DiscoveryHub.
//
// Every source only yields naming.Fields; the file name is always built by
// naming.Fields.FileName. Nothing here changes anything — a suggestion just
// fills the rename box.

// Suggestion is one proposed name.
type Suggestion struct {
	Name   string        `json:"name"`
	Source string        `json:"source"`
	Fields naming.Fields `json:"fields"`
	// Kind is TMDB's "movie"/"tv" for a TMDB match.
	Kind string `json:"kind,omitempty"`
}

const maxTMDBSuggestions = 3

type suggestRequest struct {
	Path   string `json:"path"`
	Source string `json:"source"` // metadata, tmdb, ai
}

// recordingMeta is what the suggestion sources know about one recording.
type recordingMeta struct {
	file, title, description, folder string
	folders                          []string // nearest first
}

func (h *Media) readMeta(tsPath string, roots []string) recordingMeta {
	m := recordingMeta{file: path.Base(tsPath), title: recfs.Base(tsPath)}
	if data, err := h.FS.ReadFile(tsPath + ".meta"); err == nil {
		lines := strings.SplitN(string(data), "\n", 4)
		if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
			m.title = strings.TrimSpace(lines[1])
		}
		if len(lines) > 2 {
			m.description = strings.TrimSpace(lines[2])
		}
	}
	dir := path.Dir(tsPath)
	for _, r := range roots {
		r = path.Clean(r)
		if strings.HasPrefix(dir+"/", r+"/") {
			m.folder = strings.TrimPrefix(strings.TrimPrefix(dir, r), "/")
		}
	}
	parts := strings.Split(m.folder, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			m.folders = append(m.folders, parts[i])
		}
	}
	return m
}

func (h *Media) suggest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	var req suggestRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"path", "source"}`)
		return
	}
	roots, ok := h.requireFileOps(w, r)
	if !ok {
		return
	}
	p, err := recfs.CleanIn(h.FS, req.Path, roots)
	if err == nil {
		_, err = recfs.Files(h.FS, p)
	}
	if err != nil {
		writeRecfsError(w, r, err)
		return
	}
	meta := h.readMeta(p, roots)
	base := naming.FromMetadata(meta.title, meta.description, meta.folders)

	var out []Suggestion
	add := func(f naming.Fields, source, kind string) {
		name := f.FileName()
		if name == "" || name == recfs.Base(p) {
			return // nothing to suggest
		}
		for _, s := range out {
			if s.Name == name {
				return
			}
		}
		out = append(out, Suggestion{Name: name, Source: source, Fields: f, Kind: kind})
	}

	switch req.Source {
	case "metadata":
		add(base, "metadata", "")

	case "tmdb":
		v, err := h.Settings.Values(ctx, user)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
			return
		}
		key := v.String(connector.KeyTMDBAPIKey)
		if key == "" || h.TMDB == nil {
			httpx.WriteError(w, r, http.StatusConflict, "tmdb_not_configured", "configure a TMDB API key in Settings first")
			return
		}
		lang := tmdbLanguage(v)
		results, err := h.TMDB.Search(ctx, key, base.Title, lang)
		if errors.Is(err, tmdb.ErrUnauthorized) {
			httpx.WriteError(w, r, http.StatusBadGateway, "tmdb_unauthorized", "TMDB rejected the API key")
			return
		} else if err != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "tmdb_unavailable", "could not reach TMDB")
			return
		}
		for _, res := range results {
			if len(out) >= maxTMDBSuggestions {
				break
			}
			f := naming.Fields{Title: res.Title}
			if res.Kind == "tv" && base.IsEpisode() {
				f.Season, f.Episode, f.EpisodeTitle = base.Season, base.Episode, base.EpisodeTitle
				if title, err := h.TMDB.EpisodeTitle(ctx, key, res.ID, base.Season, base.Episode, lang); err == nil && title != "" {
					f.EpisodeTitle = title
				}
			} else if res.Kind == "movie" {
				f.Year = res.Year
			}
			if f.Valid() {
				add(f, "tmdb", res.Kind)
			}
		}

	case "ai":
		v, err := h.Settings.Values(ctx, user)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
			return
		}
		key := v.String(connector.KeyOpenAIAPIKey)
		if key == "" || h.AI == nil {
			httpx.WriteError(w, r, http.StatusConflict, "ai_not_configured", "configure an OpenAI API key in Settings first")
			return
		}
		if code := reserveAICall(ctx, h.Store, v, h.Now); code != "" {
			status := http.StatusTooManyRequests
			if code == "internal" {
				status = http.StatusInternalServerError
			}
			httpx.WriteError(w, r, status, code, "the AI call can't be made right now")
			return
		}
		f, err := h.AI.SuggestName(ctx, key, v.String(connector.KeyOpenAIModel), ai.NameInput{
			FileName: meta.file, Title: meta.title, Description: meta.description, Folder: meta.folder,
		})
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "ai_unavailable", "could not get a name suggestion")
			return
		}
		add(f, "ai", "")

	default:
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `source must be "metadata", "tmdb" or "ai"`)
		return
	}
	if out == nil {
		out = []Suggestion{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"suggestions": out})
}

// tmdbLanguage maps the user's UI language to TMDB's language parameter, so
// titles come back as the user knows them ("Stirb langsam", not "Die Hard").
func tmdbLanguage(v settings.Values) string {
	return enrich.Language(v.String(settings.KeyLanguage))
}
