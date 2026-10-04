package hubs

import (
	"context"
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/naming"
	"github.com/antonbier/DreamHub/backend/internal/settings"
)

// Naming schemes (Settings → Recordings): how MediaHub names recordings
// it renames — a single rename's suggestions and the batch rename
// (media_batch.go) alike. See internal/naming/template.go for the syntax.

// Setting keys owned by MediaHub.
const (
	KeyNamingMovie   = "media.naming.movie"
	KeyNamingEpisode = "media.naming.episode"
	// Folder schemes for "sort into folders" (media_batch.go); empty =
	// leave recordings where they are.
	KeyFolderMovie   = "media.naming.folderMovie"
	KeyFolderEpisode = "media.naming.folderEpisode"
)

// MediaSettingsSection is MediaHub's own settings section.
func MediaSettingsSection() settings.Section {
	return settings.Section{
		ID:    "media",
		Label: "settings.media",
		Fields: []settings.Field{
			{Key: KeyNamingMovie, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.media.namingMovie", Default: naming.DefaultMovieTemplate},
			{Key: KeyNamingEpisode, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.media.namingEpisode", Default: naming.DefaultEpisodeTemplate},
			{Key: KeyFolderMovie, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.media.folderMovie", Default: ""},
			{Key: KeyFolderEpisode, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.media.folderEpisode", Default: ""},
		},
		Check: func(v settings.Values) map[string]settings.FieldError {
			errs := map[string]settings.FieldError{}
			for key, episode := range map[string]bool{KeyNamingMovie: false, KeyNamingEpisode: true} {
				if err := naming.CheckTemplate(v.String(key), episode); err != nil {
					errs[key] = settings.FieldError{Code: templateErrorCode(err)}
				}
			}
			for _, key := range []string{KeyFolderMovie, KeyFolderEpisode} {
				if err := naming.CheckFolderTemplate(v.String(key)); err != nil {
					errs[key] = settings.FieldError{Code: templateErrorCode(err)}
				}
			}
			if len(errs) == 0 {
				return nil
			}
			return errs
		},
	}
}

func templateErrorCode(err error) string {
	for _, e := range []error{naming.ErrTemplateSyntax, naming.ErrTemplatePlaceholder, naming.ErrTemplateNoTitle, naming.ErrTemplateNoEpisode, naming.ErrTemplateSlash, naming.ErrFolderTemplate} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	return naming.ErrTemplateSyntax.Error()
}

// templates are the current user's naming schemes; the defaults when
// settings can't be read.
func (h *Media) templates(ctx context.Context) naming.Templates {
	ts := naming.Templates{Movie: naming.DefaultMovieTemplate, Episode: naming.DefaultEpisodeTemplate}
	if h.Settings == nil {
		return ts
	}
	user, _ := httpx.User(ctx)
	if v, err := h.Settings.Values(ctx, user); err == nil {
		if s := v.String(KeyNamingMovie); s != "" {
			ts.Movie = s
		}
		if s := v.String(KeyNamingEpisode); s != "" {
			ts.Episode = s
		}
		ts.FolderMovie, ts.FolderEpisode = v.String(KeyFolderMovie), v.String(KeyFolderEpisode)
	}
	return ts
}

// extraFor is what a template can use beyond the fields: the channel (as
// the box's movie list names it, from channels — filename → service name)
// and the recording's day from .ts.meta line 4, in the box's own zone
// (a file name is shown to people, like any other local time).
func (h *Media) extraFor(tsPath string, channels map[string]string) naming.Extra {
	x := naming.Extra{Channel: channels[tsPath]}
	if data, err := readRegular(h.FS, tsPath+".meta"); err == nil {
		lines := strings.SplitN(string(data), "\n", 5)
		if len(lines) > 3 {
			if t, err := strconv.ParseInt(strings.TrimSpace(lines[3]), 10, 64); err == nil && t > 0 {
				x.Date = time.Unix(t, 0).In(time.Local)
			}
		}
	}
	return x
}

// channelsIn maps each recording in dir to its channel name, from the
// box's movie list (the .ts.meta only has the service reference).
func (h *Media) channelsIn(ctx context.Context, dir string) map[string]string {
	out := map[string]string{}
	folder, err := h.WC.MovieDir(ctx, strings.TrimSuffix(dir, "/")+"/")
	if err != nil {
		return out
	}
	for _, m := range folder.Movies {
		out[path.Clean("/"+strings.TrimLeft(m.Filename, "/"))] = m.ServiceName
	}
	return out
}
