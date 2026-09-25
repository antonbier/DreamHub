// Package naming turns what's known about a recording — title, year,
// season/episode, episode title — into a clean file name. Every name
// suggestion source (the recording's own metadata, TMDB, the AI provider)
// only supplies these Fields; the file name itself is always built here,
// from one template and one sanitizer (decision D2, docs/V1_PLAN.md). The
// AI in particular never writes a file name.
package naming

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Fields are the structured facts a name is built from. Zero values mean
// "unknown".
type Fields struct {
	Title        string `json:"title"`
	Year         int    `json:"year,omitempty"`
	Season       int    `json:"season,omitempty"`
	Episode      int    `json:"episode,omitempty"`
	EpisodeTitle string `json:"episodeTitle,omitempty"`
}

// IsEpisode reports whether the fields describe a series episode.
func (f Fields) IsEpisode() bool { return f.Episode > 0 }

// Valid checks fields from an outside source (TMDB, AI) before use.
func (f Fields) Valid() bool {
	return strings.TrimSpace(f.Title) != "" && len([]rune(f.Title)) <= 150 &&
		(f.Year == 0 || (f.Year >= 1900 && f.Year <= 2100)) &&
		f.Season >= 0 && f.Season <= 9999 && f.Episode >= 0 && f.Episode <= 99999 &&
		len([]rune(f.EpisodeTitle)) <= 150
}

// FileName builds the base name (without ".ts"):
//
//	series:  "Title - S01E07 - Episode title"   (episode title if known)
//	movie:   "Title (2018)"                      (year if known)
//
// The format is the one Plex/Jellyfin/Kodi recognize, so recordings also
// read well in those tools.
func (f Fields) FileName() string {
	var name string
	switch {
	case f.IsEpisode():
		name = fmt.Sprintf("%s - S%02dE%02d", f.Title, f.Season, f.Episode)
		if f.EpisodeTitle != "" {
			name += " - " + f.EpisodeTitle
		}
	case f.Year > 0:
		name = fmt.Sprintf("%s (%d)", f.Title, f.Year)
	default:
		name = f.Title
	}
	return Sanitize(name)
}

// maxBytes matches recfs.MaxNameBytes (kept separate so naming doesn't
// depend on the file-system package).
const maxBytes = 200

// Sanitize makes a name safe as a file name on the box *and* over SMB — the
// recordings folder is browsed from Macs/PCs over Samba (ARCHITECTURE.md),
// where / \ : * ? " < > | are not allowed. Colons become " -" ("Tatort:
// Borowski" → "Tatort - Borowski"), the rest is dropped; control
// characters and repeated spaces go; the result is cut to fit.
func Sanitize(s string) string {
	s = strings.ReplaceAll(s, ": ", " - ")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '/' || r == '\\' || r == ':':
			b.WriteString("-")
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		case strings.ContainsRune(`*?"<>|`, r), unicode.IsControl(r):
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	out = strings.Trim(out, " .-")
	for len(out) > maxBytes {
		r := []rune(out)
		out = strings.TrimRight(string(r[:len(r)-1]), " .-")
	}
	return out
}

var (
	// "S01E07", "S00E8614", case-insensitive.
	seRe = regexp.MustCompile(`(?i)\bS(\d{1,4})E(\d{1,5})\b`)
	// A year in parentheses, as SerienRecorder names series folders:
	// "Gute Zeiten, schlechte Zeiten (1992)".
	yearRe = regexp.MustCompile(`\((\d{4})\)\s*$`)
)

// FromMetadata derives fields from what the box itself knows: the title
// and description in .ts.meta (lines 2 and 3), and the folder names above
// the recording. SerienRecorder writes "<Series> - S01E07 - <Episode>" as
// the title and puts recordings under "<Series> (<year>)/Season N/"; a
// plain recording just has its broadcast title.
func FromMetadata(title, description string, folders []string) Fields {
	f := Fields{Title: strings.TrimSpace(title)}
	text := title + " " + description
	if m := seRe.FindStringSubmatchIndex(text); m != nil {
		f.Season, _ = strconv.Atoi(text[m[2]:m[3]])
		f.Episode, _ = strconv.Atoi(text[m[4]:m[5]])
		// Series title: what precedes "SxxEyy" in the title, if it's there.
		if i := seRe.FindStringIndex(title); i != nil {
			f.Title = strings.Trim(strings.TrimSpace(title[:i[0]]), " -")
			// Episode title: what follows "SxxEyy - ".
			f.EpisodeTitle = strings.Trim(strings.TrimSpace(title[i[1]:]), " -")
		}
		if f.EpisodeTitle == "" {
			if i := seRe.FindStringIndex(description); i != nil {
				f.EpisodeTitle = strings.Trim(strings.TrimSpace(firstLine(description[i[1]:])), " -")
			}
		}
	}
	// A "(year)" folder whose name starts with the title gives the year
	// (SerienRecorder's series folder, or a hand-made "Film/Heat (1995)/").
	for _, dir := range folders {
		if m := yearRe.FindStringSubmatch(dir); m != nil {
			base := strings.TrimSpace(yearRe.ReplaceAllString(dir, ""))
			if f.Title == "" || strings.EqualFold(base, f.Title) {
				f.Year, _ = strconv.Atoi(m[1])
				if f.Title == "" {
					f.Title = base
				}
				break
			}
		}
	}
	if f.IsEpisode() {
		// Series have a first-aired year on TMDB, but in a file name the
		// SxxEyy is what identifies an episode; the year only disambiguates
		// films.
		f.Year = 0
	}
	return f
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\n."); i >= 0 {
		return s[:i]
	}
	return s
}
