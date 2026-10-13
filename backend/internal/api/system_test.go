package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/settings"
)

func TestSystemBackup_ExcludesSecretsAndPasswords(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"`+settings.KeyAuthPassword+`":"a-strong-password"}}`, nil)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"`+settings.KeyLanguage+`":"de"}}`, nil)

	resp, b := e.do(t, "GET", "/api/v1/system/backup", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Version  int            `json:"version"`
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatal(err)
	}
	if body.Version != 1 {
		t.Errorf("expected backup version 1, got %d", body.Version)
	}
	if _, ok := body.Settings[settings.KeyAuthPassword]; ok {
		t.Errorf("backup must never carry a password field, got %v", body.Settings)
	}
	if got := body.Settings[settings.KeyLanguage]; got != "de" {
		t.Errorf("expected %s=%q in the backup, got %v", settings.KeyLanguage, "de", got)
	}
}

func TestSystemRestore_PreviewsThenAppliesSettings(t *testing.T) {
	e := newEnv(t)
	backupBody := `{"version":1,"settings":{"` + settings.KeyLanguage + `":"it"},"dashboard":{"tiles":[]}}`

	resp, b := e.do(t, "POST", "/api/v1/system/backup/restore", backupBody, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var preview struct {
		Plan struct {
			ID      string `json:"id"`
			Changes []struct {
				Type   string `json:"type"`
				Before any    `json:"before"`
				After  any    `json:"after"`
			} `json:"changes"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(b, &preview); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range preview.Plan.Changes {
		if c.Type == "settings."+settings.KeyLanguage {
			found = true
			if c.After != "it" {
				t.Errorf("expected after=%q, got %v", "it", c.After)
			}
		}
	}
	if !found {
		t.Fatalf("expected a settings.%s change in the preview, got %+v", settings.KeyLanguage, preview.Plan.Changes)
	}

	// Nothing changes until the plan is applied.
	_, sb := e.do(t, "GET", "/api/v1/settings", "", nil)
	if strings.Contains(string(sb), `"value":"it"`) {
		t.Fatalf("restore must not change settings before the plan is applied: %s", sb)
	}

	resp, ab := e.do(t, "POST", "/api/v1/plans/"+preview.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 applying the restore plan, got %d: %s", resp.StatusCode, ab)
	}

	_, sb2 := e.do(t, "GET", "/api/v1/settings", "", nil)
	if !strings.Contains(string(sb2), `"value":"it"`) {
		t.Fatalf("expected language=it after applying the restore, got %s", sb2)
	}
}

// --- App6: the generic backup "sections" mechanism, AppsHub's config ---------

func TestSystemBackupSections_ListsAppsWithSizes(t *testing.T) {
	e := newEnv(t)
	e.appsConfigFS.Files["/media/hdd/dreamhub-apps/pihole"] = map[string][]byte{"pihole.toml": []byte("setupVars=1")}

	resp, b := e.do(t, "GET", "/api/v1/system/backup/sections", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Sections []struct {
			ID    string `json:"id"`
			Items []struct {
				ID              string `json:"id"`
				Name            string `json:"name"`
				SizeBytes       int64  `json:"sizeBytes"`
				DefaultIncluded bool   `json:"defaultIncluded"`
			} `json:"items"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Sections) != 1 || out.Sections[0].ID != "apps" {
		t.Fatalf("got %+v, want exactly one 'apps' section", out.Sections)
	}
	var pihole *struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		SizeBytes       int64  `json:"sizeBytes"`
		DefaultIncluded bool   `json:"defaultIncluded"`
	}
	for i := range out.Sections[0].Items {
		if out.Sections[0].Items[i].ID == "pihole" {
			pihole = &out.Sections[0].Items[i]
		}
	}
	if pihole == nil || pihole.SizeBytes != int64(len("setupVars=1")) || !pihole.DefaultIncluded {
		t.Fatalf("got %+v", pihole)
	}
}

func TestSystemBackup_WithAppsSectionIncludesConfig(t *testing.T) {
	e := newEnv(t)
	e.appsConfigFS.Files["/media/hdd/dreamhub-apps/pihole"] = map[string][]byte{"pihole.toml": []byte("setupVars=1")}

	// Without ?sections=, the section is omitted entirely — an existing,
	// settings-only backup workflow is unaffected by App6.
	_, b := e.do(t, "GET", "/api/v1/system/backup", "", nil)
	var plain struct {
		Sections map[string]json.RawMessage `json:"sections"`
	}
	_ = json.Unmarshal(b, &plain)
	if plain.Sections != nil {
		t.Fatalf("got sections %v without asking for any", plain.Sections)
	}

	resp, b := e.do(t, "GET", "/api/v1/system/backup?sections=apps", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Sections map[string]json.RawMessage `json:"sections"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out.Sections["apps"]; !ok {
		t.Fatalf("got %v, want an 'apps' section", out.Sections)
	}
}

func TestSystemRestore_AppsSectionWritesConfigBack(t *testing.T) {
	e := newEnv(t)
	e.appsConfigFS.Files["/media/hdd/dreamhub-apps/pihole"] = map[string][]byte{"pihole.toml": []byte("setupVars=1")}

	_, backupBody := e.do(t, "GET", "/api/v1/system/backup?sections=apps", "", nil)

	// Simulate a fresh box: wipe what's there, then restore from the
	// downloaded file.
	e.appsConfigFS.Files = map[string]map[string][]byte{}

	resp, pb := e.do(t, "POST", "/api/v1/system/backup/restore", string(backupBody), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d: %s", resp.StatusCode, pb)
	}
	var preview struct {
		Plan struct {
			ID      string `json:"id"`
			Changes []struct {
				Type string `json:"type"`
			} `json:"changes"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(pb, &preview); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range preview.Plan.Changes {
		if c.Type == "backup.section.apps" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a backup.section.apps change in the preview, got %+v", preview.Plan.Changes)
	}

	if _, ok := e.appsConfigFS.Files["/media/hdd/dreamhub-apps/pihole"]; ok {
		t.Fatal("restore must not write anything before the plan is applied")
	}

	resp, ab := e.do(t, "POST", "/api/v1/plans/"+preview.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d: %s", resp.StatusCode, ab)
	}
	if string(e.appsConfigFS.Files["/media/hdd/dreamhub-apps/pihole"]["pihole.toml"]) != "setupVars=1" {
		t.Fatalf("got %v, want pihole.toml restored", e.appsConfigFS.Files)
	}
}

// A multi-directory app: Nextcloud's own directory holds its code
// (opt-in), its database's is in by default, its files never by default;
// ?items.apps= picks exactly the listed ones. A restored item id that
// would leave the storage root is ignored.
func TestSystemBackup_AppItems(t *testing.T) {
	e := newEnv(t)
	code, p, b := e.planInstall(t, `{"appId":"nextcloud"}`)
	if code != http.StatusOK {
		t.Skip("nextcloud can't be planned on this machine:", string(b))
	}
	e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	root := "/media/hdd/dreamhub-apps/"
	e.appsConfigFS.Files[root+"nextcloud"] = map[string][]byte{"config.php": []byte("code")}
	e.appsConfigFS.Files[root+"nextcloud.db"] = map[string][]byte{"ibdata1": []byte("rows")}
	e.appsConfigFS.Files[root+"nextcloud.data"] = map[string][]byte{"photo.jpg": []byte("pixels")}

	_, b = e.do(t, "GET", "/api/v1/system/backup/sections", "", nil)
	var secs struct {
		Sections []struct {
			Items []struct {
				ID              string `json:"id"`
				DefaultIncluded bool   `json:"defaultIncluded"`
			} `json:"items"`
		} `json:"sections"`
	}
	json.Unmarshal(b, &secs)
	got := map[string]bool{}
	for _, it := range secs.Sections[0].Items {
		got[it.ID] = it.DefaultIncluded
	}
	for id, def := range map[string]bool{"nextcloud": false, "nextcloud.db": true, "nextcloud.data": false} {
		if v, ok := got[id]; !ok || v != def {
			t.Fatalf("item %s: %v %v (%s)", id, v, ok, b)
		}
	}

	_, b = e.do(t, "GET", "/api/v1/system/backup?sections=apps", "", nil)
	var out struct {
		Sections map[string]map[string]json.RawMessage `json:"sections"`
	}
	json.Unmarshal(b, &out)
	if _, ok := out.Sections["apps"]["nextcloud.db"]; !ok || out.Sections["apps"]["nextcloud"] != nil || out.Sections["apps"]["nextcloud.data"] != nil {
		t.Fatalf("default items: %v", out.Sections["apps"])
	}

	_, b = e.do(t, "GET", "/api/v1/system/backup?sections=apps&items.apps=nextcloud,nextcloud.data", "", nil)
	out.Sections = nil
	json.Unmarshal(b, &out)
	if len(out.Sections["apps"]) != 2 || out.Sections["apps"]["nextcloud.data"] == nil {
		t.Fatalf("picked items: %v", out.Sections["apps"])
	}
}
