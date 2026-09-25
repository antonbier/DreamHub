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
