package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func TestBoxSettings_Overview(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/boxsettings", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d: %s", resp.StatusCode, b)
	}
	var body struct {
		Available    bool   `json:"available"`
		Writes       bool   `json:"writes"`
		BoxLevel     int    `json:"boxLevel"`
		BoxLevelName string `json:"boxLevelName"`
		Sections     []struct {
			Key           string `json:"key"`
			Count         int    `json:"count"`
			HiddenByLevel int    `json:"hiddenByLevel"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatal(err)
	}
	if !body.Available || body.Writes || body.BoxLevel != 1 || body.BoxLevelName != "Advanced" || len(body.Sections) != 5 {
		t.Fatalf("overview: %s", b)
	}
	if body.Sections[0].Key != "usage" || body.Sections[0].Count != 5 || body.Sections[0].HiddenByLevel != 1 {
		t.Errorf("usage section: %+v", body.Sections[0])
	}
}

func TestBoxSettings_SectionCarriesMarksAndHelp(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/boxsettings/sections/vti_user_interface", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d: %s", resp.StatusCode, b)
	}
	var body struct {
		Items []struct {
			Path       string `json:"path"`
			Kind       string `json:"kind"`
			Level      int    `json:"level"`
			GuiRestart bool   `json:"guiRestart"`
			Help       string `json:"help"`
		} `json:"items"`
	}
	json.Unmarshal(b, &body)
	if len(body.Items) != 4 || body.Items[0].Kind != "heading" {
		t.Fatalf("items: %s", b)
	}
	mode := body.Items[2]
	if mode.Path != "config.usage.servicelist_mode" || !mode.GuiRestart || mode.Level != 1 || !strings.Contains(mode.Help, "channel list") {
		t.Errorf("servicelist mode: %+v", mode)
	}

	_, b = e.do(t, "GET", "/api/v1/boxsettings/sections/avsetup", "", nil)
	if !strings.Contains(string(b), `"risk":"A"`) || !strings.Contains(string(b), `"riskCategory":"output"`) {
		t.Errorf("the A/V item carries no risk mark: %s", b)
	}
}

func TestBoxSettings_NoSecretLeaves(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/boxsettings/sections/parental", "", nil)
	if strings.Contains(string(b), "0000") || !strings.Contains(string(b), `"secret":true`) || !strings.Contains(string(b), `"isSet":true`) {
		t.Errorf("a PIN value is in the answer: %s", b)
	}
	_, b = e.do(t, "GET", "/api/v1/boxsettings/search?q=pin", "", nil)
	if strings.Contains(string(b), "0000") {
		t.Errorf("a PIN value is in the search: %s", b)
	}
}

func TestBoxSettings_SearchAndUnknownSection(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/boxsettings/search?q=margin", "", nil)
	var body struct {
		Available bool `json:"available"`
		Hits      []struct {
			SectionKey string `json:"sectionKey"`
			Item       struct {
				Path string `json:"path"`
			} `json:"item"`
		} `json:"hits"`
	}
	json.Unmarshal(b, &body)
	if !body.Available || len(body.Hits) != 2 || body.Hits[0].SectionKey != "usage" {
		t.Errorf("search: %s", b)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/boxsettings/search?q=", "", nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("blank query: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/boxsettings/sections/nope", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown section: %d", resp.StatusCode)
	}
}

func TestBoxSettings_ABoxWithoutSetupMenus(t *testing.T) {
	m := mock.New()
	m.NoBoxSettings = true
	e := newEnvWith(t, m)
	_, b := e.do(t, "GET", "/api/v1/boxsettings", "", nil)
	if !strings.Contains(string(b), `"available":false`) {
		t.Errorf("overview: %s", b)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/boxsettings/sections/usage", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("section: %d", resp.StatusCode)
	}
	_, b = e.do(t, "GET", "/api/v1/boxsettings/search?q=margin", "", nil)
	if !strings.Contains(string(b), `"available":false`) {
		t.Errorf("search: %s", b)
	}
}

func TestBoxSettings_ATreeSectionIsMarkedAndItsStateAndSecretsToo(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/boxsettings", "", nil)
	var ov struct {
		Sections []struct{ Key, Group string } `json:"sections"`
	}
	json.Unmarshal(b, &ov)
	if ov.Sections[4].Key != "tree.settings.plugins.demo" || ov.Sections[4].Group != "tree" || ov.Sections[0].Group != "" {
		t.Fatalf("groups: %s", b)
	}
	_, b = e.do(t, "GET", "/api/v1/boxsettings/sections/tree.settings.plugins.demo", "", nil)
	var sec struct {
		Items []struct {
			Path   string `json:"path"`
			Tree   bool   `json:"tree"`
			State  bool   `json:"state"`
			Secret bool   `json:"secret"`
			IsSet  bool   `json:"isSet"`
			Value  any    `json:"value"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &sec); err != nil || len(sec.Items) != 4 {
		t.Fatalf("tree section: %s", b)
	}
	for _, it := range sec.Items {
		if !it.Tree {
			t.Errorf("%s is not marked as a tree item", it.Path)
		}
		switch it.Path {
		case "config.plugins.demo.lastscan":
			if !it.State {
				t.Error("lastscan is not marked as state")
			}
		case "config.plugins.demo.password":
			if !it.Secret || !it.IsSet || it.Value != nil {
				t.Errorf("password: %+v", it)
			}
		case "config.plugins.demo.enabled":
			if it.State || it.Secret {
				t.Errorf("enabled: %+v", it)
			}
		}
	}
}

func TestBoxSettings_ADreamboxWithAnOldPluginSaysSo(t *testing.T) {
	m := mock.New()
	m.OldSettingsPlugin = true
	e := newEnvWith(t, m)
	_, b := e.do(t, "GET", "/api/v1/boxsettings", "", nil)
	if !strings.Contains(string(b), `"available":false`) || !strings.Contains(string(b), `"reason":"plugin"`) {
		t.Errorf("overview: %s", b)
	}
	m2 := mock.New()
	m2.NoBoxSettings = true
	_, b = newEnvWith(t, m2).do(t, "GET", "/api/v1/boxsettings", "", nil)
	if !strings.Contains(string(b), `"reason":"unsupported"`) {
		t.Errorf("unsupported: %s", b)
	}
}
