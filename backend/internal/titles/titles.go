// Package titles compares broadcast, recording and series titles the way
// a person would: case, common accents and punctuation don't matter, and
// "Tatort: Borowski …" is an episode of "Tatort". Shared by SeriesHub,
// DiscoveryHub and enrichment, so they all agree on what "the same title"
// means.
package titles

import (
	"regexp"
	"strings"
	"unicode"
)

// Normalize lowercases, folds common accents and replaces every run of
// non-letters/digits with one space, so "Die Verräter – Vertraue
// Niemandem!" and "die verrater vertraue niemandem" compare equal.
func Normalize(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		r = foldAccent(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		} else {
			space = true
		}
	}
	return b.String()
}

func foldAccent(r rune) rune {
	switch r {
	case 'ä', 'à', 'á', 'â':
		return 'a'
	case 'ö', 'ò', 'ó', 'ô':
		return 'o'
	case 'ü', 'ù', 'ú', 'û':
		return 'u'
	case 'é', 'è', 'ê':
		return 'e'
	case 'í', 'ì', 'î':
		return 'i'
	}
	return r
}

// separators split a series name from an episode title.
var separators = []string{": ", " - ", " – ", " — ", " | "}

// SeriesPart is the part of a title before the first episode separator
// ("Tatort: Borowski …" → "Tatort"), or the whole title.
func SeriesPart(title string) string {
	cut := len(title)
	for _, sep := range separators {
		if i := strings.Index(title, sep); i > 0 && i < cut {
			cut = i
		}
	}
	return title[:cut]
}

// Matches reports whether title is the series itself: the same name, or
// the name followed by a separator and an episode title ("Tatort: …",
// "Bones - …", "The Rookie (5)"). A longer title that merely starts with
// the same word ("Tatort-Reiniger") doesn't match.
func Matches(title, series string) bool {
	t, s := Normalize(title), Normalize(series)
	if s == "" {
		return false
	}
	if t == s {
		return true
	}
	for _, sep := range []string{":", " - ", " – ", " — ", " | ", " ("} {
		if i := strings.Index(title, sep); i > 0 && Normalize(title[:i]) == s {
			return true
		}
	}
	return false
}

// fromFileName matches Enigma2's default recording file name,
// "YYYYMMDD HHMM - Channel - Title.ts" (the box shows that as the title
// when a recording has no .ts.meta title — six such on the reference box).
var fromFileName = regexp.MustCompile(`^\d{8} \d{4} - [^-]+? - (.+?)(?:\.ts)?$`)

// FromFileName returns the title part of an Enigma2 default file name
// ("…Angel Has Fallen.ts" → "Angel Has Fallen"), with the "_ " Enigma2
// writes for a colon turned back into ": "; any other string unchanged.
// Only the title is taken — never a time (PRINCIPLES.md: no times from
// file names).
func FromFileName(s string) string {
	m := fromFileName.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return s
	}
	return strings.ReplaceAll(m[1], "_ ", ": ")
}
