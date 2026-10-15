package pkginfo

import (
	"slices"
	"testing"
)

func TestEntriesAreComplete(t *testing.T) {
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	for name, e := range entries {
		for _, l := range Languages {
			if e.Description[l] == "" {
				t.Errorf("%s: description missing for %s", name, l)
			}
		}
		if e.Category != "" && !ValidCategory(e.Category) {
			t.Errorf("%s: unknown category %q", name, e.Category)
		}
	}
	for _, c := range Categories {
		for _, l := range Languages {
			if c.Label[l] == "" {
				t.Errorf("category %s: label missing for %s", c.ID, l)
			}
		}
	}
}

func TestCategoryOf(t *testing.T) {
	cases := map[string]string{
		"enigma2-plugin-extensions-mediaportal":        "extensions",
		"enigma2-plugin-skins-metrix":                  "skins",
		"enigma2-plugin-skincomponents-dreamhubbutton": "skins",
		"enigma2-plugin-systemplugins-streamservices":  "systemplugins",
		"enigma2-plugin-drivers-dvb-usb":               "drivers",
		"enigma2-plugin-picons-default":                "picons",
		"gp4-picon-hotbird13e":                         "picons",
		"kernel-module-overlay":                        "kernel",
		"htop":                                         "system",
		"enigma2":                                      "system",
	}
	for name, want := range cases {
		if got := CategoryOf(name); got != want {
			t.Errorf("CategoryOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestResolveCurated(t *testing.T) {
	r := Resolve("ffmpeg", "apt text", "de-DE")
	if r.Source != SourceCurated || r.Description == "" || r.Description == "apt text" {
		t.Fatalf("curated: %+v", r)
	}
	if !slices.Equal(r.NeededBy, []string{"media"}) {
		t.Errorf("neededBy = %v", r.NeededBy)
	}
	de, en := Resolve("ffmpeg", "", "de"), Resolve("ffmpeg", "", "en")
	if de.Description == en.Description {
		t.Error("languages should differ")
	}
	if Resolve("ffmpeg", "", "fr").Description != en.Description {
		t.Error("unknown language should fall back to en")
	}
	// A curated category wins over the name convention.
	if r := Resolve("kernel-module-overlay", "", "it"); r.Category != "kernel" || r.CategoryLabel != "Moduli del kernel" {
		t.Errorf("category: %+v", r)
	}
	if r := Resolve("enigma2-plugin-skincomponents-dreamhubbutton", "", "en"); r.Category != "skins" {
		t.Errorf("category: %+v", r)
	}
}

func TestResolveFallbackAndNone(t *testing.T) {
	r := Resolve("enigma2-plugin-extensions-unknownthing", "  Does a thing  ", "en")
	if r.Source != SourcePackage || r.Description != "Does a thing" || r.Category != "extensions" || r.CategoryLabel != "Extensions" {
		t.Errorf("package fallback: %+v", r)
	}
	r = Resolve("some-unknown-lib", "", "en")
	if r.Source != SourceNone || r.Description != "" || r.Category != "system" {
		t.Errorf("none: %+v", r)
	}
}

func TestMissing(t *testing.T) {
	got := Missing([]string{"ffmpeg", "abc", "htop", "xyz"})
	if !slices.Equal(got, []string{"abc", "xyz"}) {
		t.Errorf("Missing = %v", got)
	}
}

func TestLang(t *testing.T) {
	for in, want := range map[string]string{"de": "de", "DE_de": "de", "it-IT": "it", "en-GB": "en", "": "en", "x": "en", "fr": "en"} {
		if got := Lang(in); got != want {
			t.Errorf("Lang(%q) = %q, want %q", in, got, want)
		}
	}
}
