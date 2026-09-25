package naming

import (
	"strings"
	"testing"
)

func TestFromMetadata(t *testing.T) {
	cases := []struct {
		name, title, desc string
		folders           []string
		want              Fields
		file              string
	}{
		{
			// Real SerienRecorder recording on the reference box.
			"serienrecorder episode",
			"Gute Zeiten, schlechte Zeiten - S00E8614 - Hirngespinst", "S00E8614 - Hirngespinst",
			[]string{"Season 0", "Gute Zeiten, schlechte Zeiten (1992)"},
			Fields{Title: "Gute Zeiten, schlechte Zeiten", Season: 0, Episode: 8614, EpisodeTitle: "Hirngespinst"},
			"Gute Zeiten, schlechte Zeiten - S00E8614 - Hirngespinst",
		},
		{
			"film in a (year) folder",
			"Heat", "Thriller, USA 1995",
			[]string{"Heat (1995)", "Deutsch", "Film"},
			Fields{Title: "Heat", Year: 1995},
			"Heat (1995)",
		},
		{
			"plain broadcast with a colon",
			"Tatort: Borowski und das Haus am Meer", "Krimi",
			[]string{"movie"},
			Fields{Title: "Tatort: Borowski und das Haus am Meer"},
			"Tatort - Borowski und das Haus am Meer",
		},
		{
			"episode number only in the description",
			"Die Kanzlei", "S01E07 - Der Fall Richter. Isa und Markus…",
			nil,
			Fields{Title: "Die Kanzlei", Season: 1, Episode: 7, EpisodeTitle: "Der Fall Richter"},
			"Die Kanzlei - S01E07 - Der Fall Richter",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FromMetadata(c.title, c.desc, c.folders)
			if got != c.want {
				t.Errorf("FromMetadata = %+v, want %+v", got, c.want)
			}
			if name := got.FileName(); name != c.file {
				t.Errorf("FileName = %q, want %q", name, c.file)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		`AC/DC: Live "Wembley" <1986>?`: "AC-DC - Live Wembley 1986",
		"  too   many  spaces  ":        "too many spaces",
		"line\nbreak\ttab\x07bell":      "line break tabbell",
		"...dots and dashes - ":         "dots and dashes",
	} {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	long := Sanitize(strings.Repeat("ä", 300))
	if len(long) > maxBytes {
		t.Errorf("Sanitize must cut to %d bytes, got %d", maxBytes, len(long))
	}
}

func TestValid(t *testing.T) {
	if !(Fields{Title: "Heat", Year: 1995}).Valid() {
		t.Error("a normal film is valid")
	}
	for _, f := range []Fields{{}, {Title: "X", Year: 20}, {Title: "X", Season: -1}, {Title: strings.Repeat("x", 151)}} {
		if f.Valid() {
			t.Errorf("%+v should be invalid", f)
		}
	}
}
