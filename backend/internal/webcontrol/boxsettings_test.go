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

const bsFixtures = "../../testdata/fixtures/vti15_boxsettings/"

// solo2Settings serves what a VU+ Solo2 (VTi 15) answered on 2026-10-23.
func solo2Settings(t *testing.T) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serve := func(name, ctype string) {
			b, err := os.ReadFile(name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", ctype)
			w.Write(b)
		}
		switch {
		case r.URL.Path == "/web/deviceinfo":
			serve("../../testdata/fixtures/vti15_deviceinfo.xml", "text/xml")
		case r.URL.Path == "/api/config":
			serve(bsFixtures+"config.json", "application/json")
		case strings.HasPrefix(r.URL.Path, "/api/config/"):
			serve(bsFixtures+"sec_"+strings.TrimPrefix(r.URL.Path, "/api/config/")+".json", "application/json")
		case r.URL.Path == "/file":
			f := r.URL.Query().Get("file")
			switch f {
			case "/usr/share/enigma2/setup.xml":
				serve(bsFixtures+"setup.xml", "text/xml")
			case "/usr/lib/enigma2/python/Plugins/SystemPlugins/HDMICEC/setup.xml":
				serve(bsFixtures+"plugin_HDMICEC.xml", "text/xml")
			case "/usr/lib/enigma2/python/Plugins/SystemPlugins/AutoShutDown/setup.xml":
				serve(bsFixtures+"plugin_AutoShutDown.xml", "text/xml")
			default: // TransCodingSetup: this box is told to have none
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL)
}

func TestConfigSections(t *testing.T) {
	secs, err := solo2Settings(t).ConfigSections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 18 || secs[2].Key != "vti_movies" || secs[2].Title != "Aufnahmen" {
		t.Errorf("sections: %+v", secs)
	}
}

func TestConfigItems(t *testing.T) {
	c := solo2Settings(t)
	items, err := c.ConfigItems(context.Background(), "usage")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ConfigItem{}
	for _, it := range items {
		by[it.Path] = it
	}
	level := by["config.usage.setup_level"]
	if level.Kind != KindChoice || level.Value != "expert" || len(level.Choices) != 3 || level.Choices[0] != (ConfigChoice{Value: "simple", Label: "Einfach"}) {
		t.Errorf("setup level: %+v", level)
	}
	if m := by["config.recording.margin_before"]; m.Kind != KindNumber || m.Value != float64(0) {
		t.Errorf("margin: %+v", m)
	}
	if b := by["config.usage.show_message_when_recording_starts"]; b.Kind != KindBool || b.Value != true {
		t.Errorf("bool: %+v", b)
	}
	sp, err := c.ConfigItems(context.Background(), "vti_movies")
	if err != nil {
		t.Fatal(err)
	}
	headings := 0
	for _, it := range sp {
		if it.Kind == KindHeading {
			headings++
			if it.Value != nil || it.Label == "" {
				t.Errorf("heading: %+v", it)
			}
		}
	}
	if len(sp) != 68 || headings < 2 {
		t.Errorf("vti_movies: %d items, %d headings", len(sp), headings)
	}
	seek, err := c.ConfigItems(context.Background(), "usage")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range seek {
		if it.Path == "config.seek.speeds_forward" {
			list, ok := it.Value.([]any)
			if it.Kind != KindMultiChoice || !ok || len(list) != 7 || len(it.Choices) != 12 || it.Choices[0] != (ConfigChoice{Value: "2", Label: "2"}) {
				t.Errorf("multichoice: %+v", it)
			}
		}
	}
}

func TestConfigItemsRefusesAnOddKey(t *testing.T) {
	for _, k := range []string{"", "../x", "a/b", "a b", strings.Repeat("a", 65)} {
		if _, err := solo2Settings(t).ConfigItems(context.Background(), k); err == nil {
			t.Errorf("key %q was accepted", k)
		}
	}
}

func TestSetupDefinition(t *testing.T) {
	defs, err := solo2Settings(t).SetupDefinition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]SetupItem{}
	sections := map[string]SetupSection{}
	for _, s := range defs {
		sections[s.Key] = s
		for _, it := range s.Items {
			if _, ok := items[it.Path]; !ok {
				items[it.Path] = it
			}
		}
	}
	// system file (21 sections) plus the two plugin files this box serves
	if len(defs) != 23 || !sections["vti_movies"].OpenWebif || sections["usage"].OpenWebif || sections["hdmi_cec"].Title != "HDMI CEC" {
		t.Errorf("sections: %d %+v", len(defs), sections["usage"])
	}
	if _, ok := sections["transcoding"]; ok {
		t.Error("a plugin file that answered 404 produced a section")
	}
	gui, withDesc := 0, 0
	for _, s := range defs {
		for _, it := range s.Items {
			if it.GuiRestart {
				gui++
			}
			if it.Description != "" {
				withDesc++
			}
		}
	}
	if gui != 44 || withDesc != 75 {
		t.Errorf("guirestart %d (want 44), description %d (want 75)", gui, withDesc)
	}
	if it := items["config.recording.margin_before"]; it.Level != 0 || it.Requires != "PVRSupport" || it.Text != "Margin before record (minutes)" {
		t.Errorf("margin item: %+v", it)
	}
	if it := items["config.av.downmix_ac3"]; it.Level != 1 || it.Requires != "CanDownmixAC3" {
		t.Errorf("downmix item: %+v", it)
	}
}

func TestBoxSettingsOnADreamboxWithoutThePlugin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// a Dreambox: WebControl's own deviceinfo, no /api/config, no plugin route
		if r.URL.Path == "/web/deviceinfo" {
			b, _ := os.ReadFile("../../testdata/fixtures/deviceinfo.xml")
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	if _, err := c.ConfigSections(context.Background()); !errors.Is(err, ErrNeedsPlugin) {
		t.Errorf("sections: %v", err)
	}
	if _, err := c.ConfigItems(context.Background(), "usage"); !errors.Is(err, ErrNeedsPlugin) {
		t.Errorf("items: %v", err)
	}
	if _, err := c.SetupDefinition(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Errorf("setup: %v", err)
	}
}
