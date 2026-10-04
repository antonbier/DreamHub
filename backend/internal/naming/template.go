package naming

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Templates are the user's naming schemes (Settings → Recordings): one for
// films and one for series episodes. A template is text with placeholders
// in braces and optional parts in square brackets:
//
//	{title}[ ({year})]
//	{title} - S{season:02}E{episode:02}[ - {episodeTitle}]
//
// Placeholders: {title}, {year}, {season}, {episode}, {episodeTitle},
// {channel}, {date} (the recording's day, YYYY-MM-DD). Numbers take an
// optional zero-padding width: {season:02}. A bracketed part is left out
// entirely when any placeholder in it is empty (or 0) — so "( )" or a
// dangling " - " never ends up in a name. The result goes through
// Sanitize like every other name.
type Templates struct {
	Movie   string
	Episode string
	// Folder schemes (optional, "" = stay in the folder): where a batch
	// rename sorts films and episodes into, relative to the recording
	// folder — e.g. "{title}/Staffel {season:02}". See RenderFolder.
	FolderMovie   string
	FolderEpisode string
}

// Folder is the relative folder f goes into by the matching folder
// scheme, "" for none.
func (ts Templates) Folder(f Fields, x Extra) string {
	if f.IsEpisode() {
		return RenderFolder(ts.FolderEpisode, f, x)
	}
	return RenderFolder(ts.FolderMovie, f, x)
}

// Default templates: exactly what Fields.FileName has always produced.
const (
	DefaultMovieTemplate   = "{title}[ ({year})]"
	DefaultEpisodeTemplate = "{title} - S{season:02}E{episode:02}[ - {episodeTitle}]"
)

// Extra is what a template can use beyond Fields.
type Extra struct {
	Channel string
	Date    time.Time // zero: unknown
}

var placeholders = map[string]bool{
	"title": true, "year": true, "season": true, "episode": true, "episodeTitle": true, "channel": true, "date": true,
}

var numeric = map[string]bool{"year": true, "season": true, "episode": true}

// Template errors, as settings field error codes.
var (
	ErrTemplateSyntax      = errors.New("template_syntax")      // unbalanced or nested brackets/braces
	ErrTemplatePlaceholder = errors.New("template_placeholder") // an unknown placeholder
	ErrTemplateNoTitle     = errors.New("template_no_title")    // {title} must appear, outside brackets
	ErrTemplateNoEpisode   = errors.New("template_no_episode")  // an episode template needs {episode}
	ErrTemplateSlash       = errors.New("template_slash")       // no folders: names only
)

type token struct {
	text     string // literal text, or
	name     string // a placeholder
	width    int    // zero-padding width for numbers
	optional int    // index of the bracketed group, -1 outside
}

func parse(tpl string) ([]token, error) {
	if strings.ContainsAny(tpl, "/\\") {
		return nil, ErrTemplateSlash
	}
	var out []token
	group, groups := -1, 0
	for i := 0; i < len(tpl); {
		switch c := tpl[i]; c {
		case '[':
			if group >= 0 {
				return nil, ErrTemplateSyntax
			}
			group = groups
			groups++
			i++
		case ']':
			if group < 0 {
				return nil, ErrTemplateSyntax
			}
			group = -1
			i++
		case '{':
			end := strings.IndexByte(tpl[i:], '}')
			if end < 0 {
				return nil, ErrTemplateSyntax
			}
			spec := tpl[i+1 : i+end]
			name, pad, hasPad := strings.Cut(spec, ":")
			if !placeholders[name] {
				return nil, fmt.Errorf("%w: %s", ErrTemplatePlaceholder, name)
			}
			t := token{name: name, optional: group}
			if hasPad {
				w, err := strconv.Atoi(pad)
				if err != nil || !numeric[name] || w < 1 || w > 5 {
					return nil, fmt.Errorf("%w: %s", ErrTemplatePlaceholder, spec)
				}
				t.width = w
			}
			out = append(out, t)
			i += end + 1
		case '}':
			return nil, ErrTemplateSyntax
		default:
			j := i
			for j < len(tpl) && !strings.ContainsRune("[]{}", rune(tpl[j])) {
				j++
			}
			out = append(out, token{text: tpl[i:j], optional: group})
			i = j
		}
	}
	if group >= 0 {
		return nil, ErrTemplateSyntax
	}
	return out, nil
}

// CheckTemplate validates a template; episode says whether it's the
// episode template (which must contain {episode}).
func CheckTemplate(tpl string, episode bool) error {
	toks, err := parse(tpl)
	if err != nil {
		return err
	}
	title, ep := false, false
	for _, t := range toks {
		if t.name == "title" && t.optional < 0 {
			title = true
		}
		if t.name == "episode" {
			ep = true
		}
	}
	if !title {
		return ErrTemplateNoTitle
	}
	if episode && !ep {
		return ErrTemplateNoEpisode
	}
	return nil
}

func value(name string, width int, f Fields, x Extra) string {
	n := 0
	switch name {
	case "title":
		return f.Title
	case "episodeTitle":
		return f.EpisodeTitle
	case "channel":
		return x.Channel
	case "date":
		if x.Date.IsZero() {
			return ""
		}
		return x.Date.Format("2006-01-02")
	case "year":
		n = f.Year
	case "season":
		n = f.Season
	case "episode":
		n = f.Episode
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%0*d", width, n)
}

// Render builds a file name (without extension) from f with the template
// that fits it — the episode one when f is an episode. An invalid template
// falls back to the default, so a bad setting can't produce broken names.
func (ts Templates) Render(f Fields, x Extra) string {
	tpl, def := ts.Movie, DefaultMovieTemplate
	if f.IsEpisode() {
		tpl, def = ts.Episode, DefaultEpisodeTemplate
	}
	toks, err := parse(tpl)
	if err != nil || CheckTemplate(tpl, f.IsEpisode()) != nil {
		toks, _ = parse(def)
	}
	return render(toks, f, x)
}

// render joins a parsed template's parts, dropping a bracketed group when
// any of its placeholders is empty, and sanitizes the result.
func render(toks []token, f Fields, x Extra) string {
	dropped := map[int]bool{}
	for _, t := range toks {
		if t.name != "" && t.optional >= 0 && value(t.name, t.width, f, x) == "" {
			dropped[t.optional] = true
		}
	}
	var b strings.Builder
	for _, t := range toks {
		if t.optional >= 0 && dropped[t.optional] {
			continue
		}
		if t.name != "" {
			b.WriteString(value(t.name, t.width, f, x))
		} else {
			b.WriteString(t.text)
		}
	}
	return Sanitize(b.String())
}

// ErrFolderTemplate is a folder scheme that could leave the recording
// folder or has an empty segment.
var ErrFolderTemplate = errors.New("template_folder")

// CheckFolderTemplate validates a folder scheme ("{title}/Staffel
// {season:02}"): "" (don't move) or segments separated by "/", each a
// template like a name scheme's but without the {title} requirement. No
// leading slash, no "." or ".." segment — a folder scheme only ever goes
// deeper into the recording folder.
func CheckFolderTemplate(tpl string) error {
	if strings.TrimSpace(tpl) == "" {
		return nil
	}
	if strings.HasPrefix(tpl, "/") || strings.Contains(tpl, "\\") {
		return ErrFolderTemplate
	}
	for _, seg := range strings.Split(tpl, "/") {
		t := strings.TrimSpace(seg)
		if t == "" || t == "." || t == ".." {
			return ErrFolderTemplate
		}
		if _, err := parse(seg); err != nil {
			return err
		}
	}
	return nil
}

// RenderFolder builds the relative folder path a recording goes into by a
// folder scheme, "" for no move (an empty or invalid scheme). Each
// segment is rendered like a name and sanitized; a segment that comes out
// empty (every placeholder in it unknown) is left out.
func RenderFolder(tpl string, f Fields, x Extra) string {
	if CheckFolderTemplate(tpl) != nil || strings.TrimSpace(tpl) == "" {
		return ""
	}
	var out []string
	for _, seg := range strings.Split(tpl, "/") {
		toks, err := parse(seg)
		if err != nil {
			return ""
		}
		if s := render(toks, f, x); s != "" && s != "." && s != ".." {
			out = append(out, s)
		}
	}
	return strings.Join(out, "/")
}
