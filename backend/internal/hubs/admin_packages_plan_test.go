package hubs

import (
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/apt"
)

// The real cascade seen on the box (ARCHITECTURE.md, "Box checks for
// 0.4.0"): asking to remove one skin component removes two skins too.
func TestPackageWarnings_Cascade(t *testing.T) {
	steps := []apt.Step{
		{Kind: "remove", Name: "mediaportal-skin-shadow-wqhd"},
		{Kind: "remove", Name: "enigma2-skin-gp4-shadow-wqhd"},
		{Kind: "remove", Name: "enigma2-plugin-skincomponents-extepgshowhide"},
	}
	w := packageWarnings(steps, []string{"enigma2-plugin-skincomponents-extepgshowhide"})
	if len(w) != 2 || w[0].Code != CodePackageAlsoRemoves || w[0].Params["count"] != 2 || w[1].Code != CodePackageTouchesEnigma2 {
		t.Fatalf("got %+v", w)
	}
	if w := packageWarnings([]apt.Step{{Kind: "install", Name: "htop"}}, []string{"htop"}); len(w) != 0 {
		t.Fatalf("a console tool needs no warning: %+v", w)
	}
}

// Removing ffmpeg would take AdScan and trimming with it.
func TestPackageWarnings_NeededByHub(t *testing.T) {
	w := packageWarnings([]apt.Step{{Kind: "remove", Name: "ffmpeg"}}, []string{"ffmpeg"})
	if len(w) != 1 || w[0].Code != CodePackageNeededByHub || w[0].Params["hubs"] != "media" {
		t.Fatalf("got %+v", w)
	}
	if w := packageWarnings([]apt.Step{{Kind: "upgrade", Name: "ffmpeg"}}, []string{"ffmpeg"}); len(w) != 0 {
		t.Fatalf("upgrading isn't removing: %+v", w)
	}
}
