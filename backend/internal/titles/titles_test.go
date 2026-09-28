package titles

import "testing"

func TestFromFileName(t *testing.T) {
	for in, want := range map[string]string{
		"20230423 2115 - ORF1 HD - Angel Has Fallen.ts":                      "Angel Has Fallen",
		"20230921 0052 - ProSieben - Uncovered_ Wie gefährlich ist es.ts":    "Uncovered: Wie gefährlich ist es",
		"20230409 2142 - RAS SRF zwei HD - First Man - Aufbruch zum Mond.ts": "First Man - Aufbruch zum Mond",
		"Crazy Rich":       "Crazy Rich",
		"Oskars Kleid.mkv": "Oskars Kleid.mkv",
	} {
		if got := FromFileName(in); got != want {
			t.Errorf("FromFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatches(t *testing.T) {
	if !Matches("Tatort: Borowski", "Tatort") || Matches("Tatort-Reiniger", "Tatort") {
		t.Error("Matches")
	}
}
