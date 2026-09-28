// Package titles compares broadcast, recording and series titles the way
// a person would: case, common accents and punctuation don't matter, and
// "Tatort: Borowski …" is an episode of "Tatort". Shared by SeriesHub,
// DiscoveryHub and enrichment, so they all agree on what "the same title"
// means.
package titles

import (
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
