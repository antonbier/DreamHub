package naming

import (
	"errors"
	"testing"
	"time"
)

func TestTemplates_DefaultsMatchFileName(t *testing.T) {
	def := Templates{Movie: DefaultMovieTemplate, Episode: DefaultEpisodeTemplate}
	for _, f := range []Fields{
		{Title: "Crazy Rich", Year: 2018},
		{Title: "Crazy Rich"},
		{Title: "Tatort", Season: 2026, Episode: 31, EpisodeTitle: "Borowski und der gute Mensch"},
		{Title: "Die Kanzlei", Season: 1, Episode: 7},
		{Title: "Film: Der Anfang", Year: 1999},
	} {
		if got, want := def.Render(f, Extra{}), f.FileName(); got != want {
			t.Errorf("%+v: template %q, FileName %q", f, got, want)
		}
	}
}

func TestTemplates_CustomSchemes(t *testing.T) {
	day := time.Date(2026, 9, 20, 20, 15, 0, 0, time.UTC)
	ts := Templates{
		Movie:   "{title}[ - {year}][ - {channel}]",
		Episode: "{title} {season}x{episode:03}[ ({date})]",
	}
	if got := ts.Render(Fields{Title: "Heat", Year: 1995}, Extra{Channel: "ARD"}); got != "Heat - 1995 - ARD" {
		t.Errorf("movie: %q", got)
	}
	if got := ts.Render(Fields{Title: "Heat"}, Extra{}); got != "Heat" {
		t.Errorf("empty optional parts are dropped: %q", got)
	}
	if got := ts.Render(Fields{Title: "Bones", Season: 3, Episode: 7}, Extra{Date: day}); got != "Bones 3x007 (2026-09-20)" {
		t.Errorf("episode: %q", got)
	}
	// A broken template falls back to the default instead of producing a
	// broken name.
	bad := Templates{Movie: "{title", Episode: "{title}"}
	if got := bad.Render(Fields{Title: "Heat", Year: 1995}, Extra{}); got != "Heat (1995)" {
		t.Errorf("fallback: %q", got)
	}
}

func TestCheckTemplate(t *testing.T) {
	for tpl, want := range map[string]error{
		"{title}[ ({year})]":        ErrTemplateNoEpisode, // as an episode template
		"{title} - {episode:02}":    nil,
		"[{title}] - {episode}":     ErrTemplateNoTitle,
		"{year}":                    ErrTemplateNoTitle,
		"{title} {nope}":            ErrTemplatePlaceholder,
		"{title} {title:02}":        ErrTemplatePlaceholder,
		"{title} [[{year}]]":        ErrTemplateSyntax,
		"{title} ({year} {episode}": nil, // a lone "(" is just text
		"{title} [({year})":         ErrTemplateSyntax,
		"{title}/{year}":            ErrTemplateSlash,
		"{title} S{season}":         ErrTemplateNoEpisode,
	} {
		if err := CheckTemplate(tpl, true); !errors.Is(err, want) {
			if !(want == nil && err == nil) {
				t.Errorf("%q: want %v, got %v", tpl, want, err)
			}
		}
	}
	if err := CheckTemplate("{title}[ ({year})]", false); err != nil {
		t.Errorf("a movie template needs no {episode}: %v", err)
	}
}
