package webcontrol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const dbFixtures = "../../testdata/fixtures/dreambox_boxsettings/"

// dreamboxWithPlugin serves what a Dreambox One answered on 2026-10-23: its
// own deviceinfo, the DreamHub plugin's /dreamhub/settings/* and setup.xml
// through /file. A route the test names in missing is a 404 (an old plugin).
func dreamboxWithPlugin(t *testing.T, missing ...string) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serve := func(name string) {
			b, err := os.ReadFile(name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Write(b)
		}
		for _, m := range missing {
			if strings.HasPrefix(r.URL.Path, m) {
				http.NotFound(w, r)
				return
			}
		}
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/web/deviceinfo":
			serve("../../testdata/fixtures/deviceinfo.xml")
		case r.URL.Path == "/dreamhub/settings/info":
			serve(dbFixtures + "info.json")
		case r.URL.Path == "/dreamhub/settings/sections":
			serve(dbFixtures + "sections.json")
		case r.URL.Path == "/dreamhub/settings/roots":
			serve(dbFixtures + "roots_" + q.Get("store") + ".json")
		case r.URL.Path == "/dreamhub/settings/items" && q.Get("section") != "":
			serve(dbFixtures + "items_" + q.Get("section") + ".json")
		case r.URL.Path == "/dreamhub/settings/items" && q.Get("root") == "config.plugins.epgrefresh":
			serve(dbFixtures + "tree_epgrefresh.json")
		case r.URL.Path == "/dreamhub/settings/items" && q.Get("root") == "config.gemininetcast" && q.Get("store") == "gp":
			serve(dbFixtures + "tree_gp_gemininetcast.json")
		case r.URL.Path == "/dreamhub/settings/items" && q.Get("root") == "config.ParentalControl":
			serve(dbFixtures + "tree_parental.json")
		case r.URL.Path == "/file" && q.Get("file") == "/usr/share/enigma2/setup.xml":
			serve(dbFixtures + "setup.xml")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL)
}

func TestDreambox_Sections(t *testing.T) {
	secs, err := dreamboxWithPlugin(t).ConfigSections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	menus, tree, gp := 0, 0, 0
	keys := map[string]ConfigSection{}
	for _, s := range secs {
		keys[s.Key] = s
		switch {
		case s.Group == "":
			menus++
		case strings.HasPrefix(s.Key, "tree.gp."):
			gp++
		default:
			tree++
		}
	}
	if menus != 10 || tree != 73 || gp != 29 {
		t.Errorf("sections: %d menus, %d tree, %d gemini", menus, tree, gp)
	}
	if s := keys["tree.settings.plugins.epgrefresh"]; s.Title != "plugins.epgrefresh" || s.Group != GroupTree {
		t.Errorf("epgrefresh: %+v", s)
	}
	if s := keys["tree.gp.gemininetcast"]; s.Title != "Gemini · gemininetcast" {
		t.Errorf("gemini: %+v", s)
	}
	if _, ok := keys["usage"]; !ok {
		t.Error("the usage menu is missing")
	}
}

func TestDreambox_MenuItems(t *testing.T) {
	c := dreamboxWithPlugin(t)
	items, err := c.ConfigItems(context.Background(), "usage")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ConfigItem{}
	for _, it := range items {
		by[it.Path] = it
	}
	level := by["config.usage.setup_level"]
	if level.Kind != KindChoice || level.Value != "expert" || len(level.Choices) != 3 {
		t.Errorf("setup level: %+v", level)
	}
	if len(items) < 40 {
		t.Errorf("%d items in usage", len(items))
	}
	for _, it := range items {
		if it.Kind == KindText && it.Secret && it.Value != nil {
			t.Errorf("a secret carries a value: %+v", it)
		}
		if it.Label == "" {
			t.Errorf("no label: %+v", it)
		}
	}
	if _, err := c.ConfigItems(context.Background(), "nope"); err == nil {
		t.Error("an unknown section was found")
	}
}

func TestDreambox_TreeItems(t *testing.T) {
	c := dreamboxWithPlugin(t)
	items, err := c.ConfigItems(context.Background(), "tree.settings.plugins.epgrefresh")
	if err != nil || len(items) < 20 {
		t.Fatalf("epgrefresh: %d items, %v", len(items), err)
	}
	by := map[string]ConfigItem{}
	for _, it := range items {
		by[it.Path] = it
	}
	en := by["config.plugins.epgrefresh.enabled"]
	if en.Kind != KindBool || en.Label != "enabled" {
		t.Errorf("enabled: %+v", en)
	}
	if iv := by["config.plugins.epgrefresh.interval"]; iv.Kind != KindNumber || iv.Value == nil {
		t.Errorf("interval: %+v", iv)
	}
	// a Gemini item, from the second store
	gp, err := c.ConfigItems(context.Background(), "tree.gp.gemininetcast")
	if err != nil || len(gp) == 0 || !strings.HasPrefix(gp[0].Path, "config.gemininetcast.") {
		t.Fatalf("gemini: %d %v", len(gp), err)
	}
	if gp[0].Label != strings.TrimPrefix(gp[0].Path, "config.gemininetcast.") {
		t.Errorf("a tree label is the key name: %+v", gp[0])
	}
}

func TestDreambox_SecretsComeWithoutValues(t *testing.T) {
	items, err := dreamboxWithPlugin(t).ConfigItems(context.Background(), "tree.settings.ParentalControl")
	if err != nil {
		t.Fatal(err)
	}
	secrets := 0
	for _, it := range items {
		if it.Secret {
			secrets++
			if it.Value != nil || len(it.Choices) > 0 {
				t.Errorf("a secret carries data: %+v", it)
			}
		}
	}
	if secrets < 4 {
		t.Errorf("%d secrets among %d items", secrets, len(items))
	}
}

func TestDreambox_SetupDefinitionIsTheBoxsOwnFile(t *testing.T) {
	defs, err := dreamboxWithPlugin(t).SetupDefinition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	var usage SetupSection
	for _, s := range defs {
		n += len(s.Items)
		if s.Key == "usage" {
			usage = s
		}
	}
	// 10 sections; headings (no path) are not items here, 87 items with one
	if len(defs) != 10 || n != 87 || len(usage.Items) < 40 {
		t.Errorf("%d sections, %d items, usage %d", len(defs), n, len(usage.Items))
	}
}

func TestDreambox_APluginWithoutTheRoute(t *testing.T) {
	c := dreamboxWithPlugin(t, "/dreamhub/settings")
	for name, call := range map[string]func() error{
		"sections": func() error { _, err := c.ConfigSections(context.Background()); return err },
		"items":    func() error { _, err := c.ConfigItems(context.Background(), "usage"); return err },
	} {
		if err := call(); !errors.Is(err, ErrNeedsPlugin) {
			t.Errorf("%s with an old plugin: %v", name, err)
		}
	}
}

func TestDreambox_TreeKeys(t *testing.T) {
	for key, want := range map[string][2]string{
		"tree.settings.plugins.autotimer": {"settings", "config.plugins.autotimer"},
		"tree.gp.geminiGlobal":            {"gp", "config.geminiGlobal"},
		"tree.settings.ci":                {"settings", "config.ci"},
	} {
		store, root, ok := parseTreeKey(key)
		if !ok || store != want[0] || root != want[1] || treeKey(store, root) != key {
			t.Errorf("%s: %q %q %v", key, store, root, ok)
		}
	}
	for _, bad := range []string{"usage", "tree.", "tree.settings", "tree..x", "tree.settings."} {
		if _, _, ok := parseTreeKey(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}
