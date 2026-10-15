package apt

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Shape of `apt-cache madison` (one line per version and feed, newest first).
const madison = ` enigma2-plugin-extensions-mediaportal | 2026100101 | http://feed.example/unstable/all Packages
 enigma2-plugin-extensions-mediaportal | 2026083001 | http://feed.example/unstable/all Packages
 enigma2-plugin-extensions-mediaportal | 2026083001 | http://mirror.example/unstable/all Packages
 enigma2-plugin-extensions-mediaportal | 2026062001 | http://feed.example/old/all Packages
`

func TestParseMadison(t *testing.T) {
	got := ParseMadison([]byte(madison))
	want := []string{"2026100101", "2026083001", "2026062001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v (each version once, in the listed order)", got, want)
	}
	if got := ParseMadison([]byte("")); len(got) != 0 {
		t.Errorf("empty: %v", got)
	}
}

func TestOlderOf_OnlyWhatDpkgOrdersBefore(t *testing.T) {
	old := versionLess
	defer func() { versionLess = old }()
	// A tiny stand-in for dpkg: numeric strings of equal length.
	versionLess = func(_ context.Context, a, b string) bool { return a < b }
	got := olderOf(context.Background(), ParseMadison([]byte(madison)), "2026083001")
	if !reflect.DeepEqual(got, []string{"2026062001"}) {
		t.Fatalf("got %v: the installed version and newer ones must not be offered", got)
	}
	// The feed only has the latest: nothing to offer.
	if got := olderOf(context.Background(), []string{"2026083001"}, "2026083001"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestValidVersion(t *testing.T) {
	for _, ok := range []string{"1.0-r1", "2026083001", "1:2.3~rc1+git4-1", "11742"} {
		if !ValidVersion(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "-o", "--force", "1 2", "1;rm", "1.0\n", "=1", strings.Repeat("1", 200)} {
		if ValidVersion(bad) {
			t.Errorf("%q must be refused: it could become an apt-get option or a second package", bad)
		}
	}
}

func TestDowngradeArgs_NameAndVersionAfterDoubleDash(t *testing.T) {
	a := downgradeArgs("oscam-cycecplus", "11700", true)
	if got := strings.Join(a[len(a)-4:], " "); got != "install --allow-downgrades -- oscam-cycecplus=11700" {
		t.Fatalf("got %q", got)
	}
	if !contains(a, "-s") {
		t.Error("the preview must be a simulation")
	}
	if contains(downgradeArgs("x1", "1", false), "-s") {
		t.Error("the apply must not be one")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestCheckDowngrade_RefusesBeforeAnythingRuns(t *testing.T) {
	if err := checkDowngrade("htop", "--force-yes"); !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("version: %v", err)
	}
	if err := checkDowngrade("-o", "1.0"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("name: %v", err)
	}
	if _, err := (Exec{}).SimulateDowngrade(context.Background(), "htop", "1 2"); !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("simulate: %v", err)
	}
	if _, err := (Exec{}).ApplyDowngrade(context.Background(), "htop", "1 2"); !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("apply: %v", err)
	}
}

// apt prints a downgrade like an upgrade; the step of the package that goes
// back is marked, and so is any other that goes back — one that moves forward
// is not.
func TestMarkDowngrade(t *testing.T) {
	old := versionLess
	defer func() { versionLess = old }()
	versionLess = func(_ context.Context, a, b string) bool { return a < b }
	out := []byte(`Inst oscam-cycecplus [11760] (11700 feed [arm64])
Inst libfoo [20] (19 feed [arm64])
Inst libbar [20] (21 feed [arm64])
`)
	got := MarkDowngrade(context.Background(), ParseSimulate(out), "oscam-cycecplus")
	if got[0].Kind != "downgrade" || got[1].Kind != "downgrade" || got[2].Kind != "upgrade" {
		t.Fatalf("got %+v", got)
	}
}
