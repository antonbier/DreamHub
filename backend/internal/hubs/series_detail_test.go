package hubs

import "testing"

func TestMatchesSeries(t *testing.T) {
	cases := []struct {
		title, series string
		want          bool
	}{
		{"Tatort", "Tatort", true},
		{"Tatort: Borowski und das Haus am Meer", "Tatort", true},
		{"Bones - Die Knochenjägerin", "Bones", true},
		{"The Rookie (5)", "The Rookie", true},
		{"Die Verräter – Vertraue Niemandem!", "Die Verräter - Vertraue Niemandem!", true},
		{"die verrater vertraue niemandem", "Die Verräter - Vertraue Niemandem!", true},
		{"Tatort-Reiniger Spezial", "Tatort", false},
		{"Tatort X-Files", "Tatort", false},
		{"The Rookie: Feds", "The Rookie", true}, // a spin-off reads like an episode title; accepted
		{"Bauer sucht Frau", "Tatort", false},
		{"Tatort", "", false},
	}
	for _, c := range cases {
		if got := matchesSeries(c.title, c.series); got != c.want {
			t.Errorf("matchesSeries(%q, %q) = %v, want %v", c.title, c.series, got, c.want)
		}
	}
}

func TestParseEpisodeCode(t *testing.T) {
	for in, want := range map[string][2]int{
		"S01E02 - Pilot":         {1, 2},
		"Folge s2026e31":         {2026, 31},
		"S3 E7":                  {3, 7},
		"Borowski und das Haus":  {0, 0},
		"ASS01E02 is not a code": {0, 0},
	} {
		s, e := parseEpisodeCode(in)
		if s != want[0] || e != want[1] {
			t.Errorf("parseEpisodeCode(%q) = %d/%d, want %d/%d", in, s, e, want[0], want[1])
		}
	}
}
