package apt

import (
	"errors"
	"reflect"
	"testing"
)

// Real apt-get -s output from the reference box (apt 1.2.12, 2026-10-12).
func TestParseSimulate_Box(t *testing.T) {
	out := []byte(`Reading package lists...
Building dependency tree...
The following packages will be upgraded:
  enigma2-plugin-skincomponents-extepgshowhide
Inst enigma2-plugin-skincomponents-extepgshowhide [1.0-r0] (1.0-r1 allcodes:download.blue-panel.com [all])
Inst htop (1.0.3-r0.0 aarch64:download.blue-panel.com [arm64])
Conf enigma2-plugin-skincomponents-extepgshowhide (1.0-r1 allcodes:download.blue-panel.com [all])
Conf htop (1.0.3-r0.0 aarch64:download.blue-panel.com [arm64])
Remv mediaportal-skin-shadow-wqhd [3.1-r0]
Remv enigma2-skin-gp4-shadow-wqhd [4.0-r39]
`)
	want := []Step{
		{Kind: "upgrade", Name: "enigma2-plugin-skincomponents-extepgshowhide", OldVersion: "1.0-r0", NewVersion: "1.0-r1"},
		{Kind: "install", Name: "htop", NewVersion: "1.0.3-r0.0"},
		{Kind: "remove", Name: "mediaportal-skin-shadow-wqhd", OldVersion: "3.1-r0"},
		{Kind: "remove", Name: "enigma2-skin-gp4-shadow-wqhd", OldVersion: "4.0-r39"},
	}
	if got := ParseSimulate(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestValidNames(t *testing.T) {
	if err := ValidNames(ActionInstall, []string{"htop", "enigma2-plugin-extensions-x", "libstdc++6"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"-o"}, {"--purge"}, {"a b"}, {"Htop"}, {"x"}, {}} {
		if err := ValidNames(ActionInstall, bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%q: got %v, want ErrInvalidName", bad, err)
		}
	}
	if err := ValidNames(ActionUpgrade, nil); err != nil {
		t.Fatalf("upgrade everything: %v", err)
	}
	if err := ValidNames("purge", []string{"htop"}); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("got %v, want ErrInvalidAction", err)
	}
}

func TestArgs_NamesAfterDoubleDash(t *testing.T) {
	got := args(ActionRemove, []string{"htop"}, true)
	want := []string{"-y", "-q", "-o", "Dpkg::Options::=--force-confold", "-o", "Dpkg::Use-Pty=0", "-s", "remove", "--", "htop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestParseDumpavail(t *testing.T) {
	out := []byte(`Package: htop
Version: 1.0.3-r0.0
Section: console/utils
Description: Interactive process viewer
 htop is an interactive process viewer.

Package: dreamhub-ext-ewelink
Version: 1.0
Section: web
X-DreamHub-Category: extension
Description: Sonoff bridge

Package: htop
Version: 1.0.2
Section: console/utils
Description: older

`)
	got := ParseDumpavail(out)
	if len(got) != 2 || got[0].Name != "dreamhub-ext-ewelink" || got[0].Category != "extension" || got[1].Version != "1.0.3-r0.0" || got[1].Description != "Interactive process viewer" {
		t.Fatalf("got %+v", got)
	}
}
