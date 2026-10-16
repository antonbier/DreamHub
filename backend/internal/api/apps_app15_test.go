package api_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type restorableList struct {
	Apps []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Custom bool   `json:"custom"`
	} `json:"apps"`
}

func (e *env) restorable(t *testing.T) restorableList {
	t.Helper()
	var v restorableList
	e.json(t, "GET", "/api/v1/apps/restorable", "", nil, 200, &v)
	return v
}

// removeApp removes an installed app (by its list name) through the plan.
func (e *env) removeApp(t *testing.T, name string) {
	t.Helper()
	row := rowOf(e.rows(t, nil), name)
	if row == nil {
		t.Fatalf("%s is not installed", name)
	}
	var rp installPlan
	e.json(t, "POST", "/api/v1/apps/installed/"+row.ID+"/remove/plan", "", nil, 200, &rp)
	e.json(t, "POST", "/api/v1/plans/"+rp.Plan.ID+"/apply", "", nil, 200, nil)
}

// The app list in a backup brings an app back on a fresh box: its stored
// choices and generated passwords come with it, and the ordinary install
// plan installs it again.
func TestAppsBackup_AppListRestoresOntoAFreshBox(t *testing.T) {
	e := newEnv(t)
	e.installAndApply(t, `{"appId":"vaultwarden"}`)
	e.installAndApply(t, customBody(edComp, ""))
	root := "/media/hdd/dreamhub-apps"
	secret := string(e.appsConfigFS.Files[root]["vaultwarden.secrets.json"])

	_, backup := e.do(t, "GET", "/api/v1/system/backup?sections=apps,appList", "", nil)
	var parsed struct {
		Sections map[string]map[string]struct {
			Root       string `json:"root"`
			Definition struct {
				AppID string `json:"appId"`
			} `json:"definition"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(backup, &parsed); err != nil {
		t.Fatal(err)
	}
	list := parsed.Sections["appList"]
	if list["vaultwarden"].Definition.AppID != "vaultwarden" || len(list) < 2 {
		t.Fatalf("app list: %s", backup)
	}
	if got := e.restorable(t); len(got.Apps) != 0 {
		t.Fatalf("installed apps are not restorable: %+v", got)
	}

	// A fresh box: the apps are gone, and so is everything stored for them.
	e.removeApp(t, "Vaultwarden")
	e.removeApp(t, "My app")
	e.appsConfigFS.Files = map[string]map[string][]byte{}

	var preview struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/system/backup/restore", string(backup), nil, 200, &preview)
	if len(e.restorable(t).Apps) != 0 {
		t.Fatal("a preview alone wrote something")
	}
	e.json(t, "POST", "/api/v1/plans/"+preview.Plan.ID+"/apply", "", nil, 200, nil)

	got := e.restorable(t)
	names := map[string]bool{}
	for _, a := range got.Apps {
		names[a.Name] = a.Custom
	}
	if _, ok := names["Vaultwarden"]; !ok || !names["My app"] {
		t.Fatalf("restorable: %+v", got)
	}
	if string(e.appsConfigFS.Files[root]["vaultwarden.secrets.json"]) != secret {
		t.Fatal("the generated passwords were not restored")
	}

	// Installed again through the normal plan; then nothing is left to restore.
	var pl installPlan
	e.json(t, "POST", "/api/v1/apps/restorable/vaultwarden/plan", "", nil, 200, &pl)
	e.json(t, "POST", "/api/v1/plans/"+pl.Plan.ID+"/apply", "", nil, 200, nil)
	if rowOf(e.rows(t, nil), "Vaultwarden") == nil {
		t.Fatal("not installed again")
	}
	for _, a := range e.restorable(t).Apps {
		if a.Name == "Vaultwarden" {
			t.Fatal("an installed app is still offered")
		}
	}
	e.json(t, "POST", "/api/v1/apps/restorable/nope/plan", "", nil, 404, nil)
	e.json(t, "POST", "/api/v1/apps/restorable/..%2Fx/plan", "", nil, 404, nil)
}

// Restoring an app list never touches an app that is installed.
func TestAppsBackup_AppListLeavesInstalledAppsAlone(t *testing.T) {
	e := newEnv(t)
	e.installAndApply(t, `{"appId":"vaultwarden"}`)
	_, backup := e.do(t, "GET", "/api/v1/system/backup?sections=appList", "", nil)
	before := string(e.appsConfigFS.Files["/media/hdd/dreamhub-apps"]["vaultwarden.app.json"])
	e.appsConfigFS.Files["/media/hdd/dreamhub-apps"]["vaultwarden.app.json"] = []byte(strings.Replace(before, `"version":1`, `"version":1,"memoryLimitMB":99`, 1))
	var preview struct{ Plan struct{ ID string } }
	e.json(t, "POST", "/api/v1/system/backup/restore", string(backup), nil, 200, &preview)
	e.json(t, "POST", "/api/v1/plans/"+preview.Plan.ID+"/apply", "", nil, 200, nil)
	if !strings.Contains(string(e.appsConfigFS.Files["/media/hdd/dreamhub-apps"]["vaultwarden.app.json"]), `"memoryLimitMB":99`) {
		t.Fatal("restore overwrote the choices of an installed app")
	}
}

// A backup carries the apps' folders and passwords: reading or saving one is
// for the admin only.
func TestSystemBackup_AdminOnly(t *testing.T) {
	e := newEnv(t)
	_, viewer, _ := e.household(t)
	v := withProfile(viewer)
	for _, c := range [][2]string{{"GET", "/api/v1/system/backup"}, {"GET", "/api/v1/system/backup/sections"},
		{"GET", "/api/v1/system/backup/files"}, {"POST", "/api/v1/system/backup/save"}} {
		if resp, _ := e.do(t, c[0], c[1], "", v); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s %s: got %d", c[0], c[1], resp.StatusCode)
		}
	}
}

// "Save on the box": the same backup, kept in the backup folder, listed,
// restored from with the normal preview, and deleted.
func TestSystemBackup_SaveOnTheBox(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"general.language":"de"}}`, nil)
	var saved struct{ File struct{ Name string } }
	e.json(t, "POST", "/api/v1/system/backup/save?sections=appList", "", nil, 200, &saved)
	name := saved.File.Name
	if _, err := os.Stat(filepath.Join(e.backupDir, name)); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(e.backupDir, name)); fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("a backup file must be private: %v", fi.Mode())
	}

	var files struct {
		Files []struct{ Name string } `json:"files"`
	}
	e.json(t, "GET", "/api/v1/system/backup/files", "", nil, 200, &files)
	if len(files.Files) != 1 || files.Files[0].Name != name {
		t.Fatalf("files: %+v", files)
	}
	resp, b := e.do(t, "GET", "/api/v1/system/backup/files/"+name, "", nil)
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"version"`) {
		t.Fatalf("download: %d %s", resp.StatusCode, b)
	}

	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"general.language":"en"}}`, nil)
	var preview struct {
		Plan struct{ Changes []struct{ Type string } }
	}
	e.json(t, "POST", "/api/v1/system/backup/files/"+name+"/restore", "", nil, 200, &preview)
	if len(preview.Plan.Changes) == 0 {
		t.Fatal("the stored file changes nothing")
	}

	// Only names the box itself made are accepted: no path gets through.
	for _, bad := range []string{"..%2Fetc%2Fpasswd", "x.json", "dreamhub-backup-1.json"} {
		e.json(t, "GET", "/api/v1/system/backup/files/"+bad, "", nil, 404, nil)
		e.json(t, "DELETE", "/api/v1/system/backup/files/"+bad, "", nil, 404, nil)
	}
	e.json(t, "DELETE", "/api/v1/system/backup/files/"+name, "", nil, 204, nil)
	e.json(t, "GET", "/api/v1/system/backup/files", "", nil, 200, &files)
	if len(files.Files) != 0 {
		t.Fatalf("not deleted: %+v", files)
	}
}
