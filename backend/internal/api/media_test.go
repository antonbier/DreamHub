package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	atmock "github.com/antonbier/DreamHub/backend/internal/autotimer/mock"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

const bauerTS = mock.Root + "20260608 2048 - RTL Television - Bauer sucht Frau International.ts"

type folderResp struct {
	Dir        string `json:"dir"`
	Parent     string `json:"parent"`
	Subfolders []struct {
		Path string `json:"path"`
		Name string `json:"name"`
	} `json:"subfolders"`
	Movies []struct {
		Title       string `json:"title"`
		Filename    string `json:"filename"`
		IsRecording bool   `json:"isRecording"`
	} `json:"movies"`
	FileOps struct {
		Available bool   `json:"available"`
		Reason    string `json:"reason"`
	} `json:"fileOps"`
}

func (e *env) folder(t *testing.T, dir string) folderResp {
	t.Helper()
	p := "/api/v1/media/movies"
	if dir != "" {
		p += "?dir=" + strings.ReplaceAll(dir, " ", "%20")
	}
	resp, b := e.do(t, "GET", p, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list %q: %d %s", dir, resp.StatusCode, b)
	}
	var f folderResp
	json.Unmarshal(b, &f)
	return f
}

func (e *env) apply(t *testing.T, id string) (*http.Response, []byte) {
	t.Helper()
	return e.do(t, "POST", "/api/v1/plans/"+id+"/apply", "", nil)
}

func (e *env) plan(t *testing.T, path, body string) (planResp, int, []byte) {
	t.Helper()
	resp, b := e.do(t, "POST", path, body, nil)
	var p planResp
	json.Unmarshal(b, &p)
	return p, resp.StatusCode, b
}

func TestMedia_BrowsesFolders(t *testing.T) {
	e := newEnv(t)
	root := e.folder(t, "")
	if root.Dir != mock.Root || root.Parent != "" || !root.FileOps.Available {
		t.Fatalf("unexpected root listing: %+v", root)
	}
	if len(root.Subfolders) != 1 || root.Subfolders[0].Name != "Tatort" {
		t.Fatalf("expected the Tatort subfolder, got %+v", root.Subfolders)
	}
	var titles []string
	for _, m := range root.Movies {
		titles = append(titles, m.Title)
	}
	// The trashed recording is not listed; the .mkv is, as a non-recording.
	if len(root.Movies) != 2 || !strings.Contains(strings.Join(titles, "|"), "Bauer sucht Frau International") {
		t.Fatalf("unexpected root movies: %v", titles)
	}

	sub := e.folder(t, root.Subfolders[0].Path)
	if sub.Parent != mock.Root || len(sub.Movies) != 1 || sub.Movies[0].Title != "Tatort: Borowski und das Haus am Meer" {
		t.Fatalf("unexpected subfolder listing: %+v", sub)
	}

	resp, b := e.do(t, "GET", "/api/v1/media/movies?dir=/etc", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusBadRequest || code != "outside_folders" {
		t.Errorf("listing outside the recording folders must be refused, got %d %s", resp.StatusCode, b)
	}
}

func TestMedia_RenamePreviewThenApply(t *testing.T) {
	e := newEnv(t)
	p, status, b := e.plan(t, "/api/v1/media/rename/plan", fmt.Sprintf(`{"path":%q,"newName":"Bauer sucht Frau International - Folge 7"}`, "/"+bauerTS))
	if status != http.StatusOK || p.Plan.Action != "media.rename" {
		t.Fatalf("plan: %d %s", status, b)
	}
	if len(p.Plan.Changes) != 7 {
		t.Fatalf("the preview must list every file of the recording (7), got %d: %s", len(p.Plan.Changes), b)
	}
	if e.folder(t, "").Movies[0].Title == "" || !strings.Contains(fmt.Sprint(e.folder(t, "").Movies), "20260608 2048") {
		t.Fatal("a preview must not change anything")
	}
	if resp, b := e.apply(t, p.Plan.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	listing := fmt.Sprint(e.folder(t, "").Movies)
	if !strings.Contains(listing, "Folge 7.ts") || strings.Contains(listing, "20260608 2048") {
		t.Errorf("the listing must show the new name: %s", listing)
	}
	_, b = e.do(t, "GET", "/api/v1/changes?limit=1", "", nil)
	if !strings.Contains(string(b), "media.rename") {
		t.Errorf("the rename must be in the change log: %s", b)
	}
}

func TestMedia_InUseIsRefused(t *testing.T) {
	// Played on the box right now.
	box := mock.New()
	box.Playing = "1:0:0:0:0:0:0:0:0:0:" + bauerTS
	e := newEnvWith(t, box)
	_, status, b := e.plan(t, "/api/v1/media/rename/plan", fmt.Sprintf(`{"path":%q,"newName":"X"}`, bauerTS))
	if code, _, _ := errCode(t, b); status != http.StatusConflict || code != "media_in_use" || !strings.Contains(string(b), `"playing"`) {
		t.Fatalf("renaming a recording being played must be refused, got %d %s", status, b)
	}

	// Being written by a running timer.
	writing := mock.New()
	e = newEnvWithOptions(t, recordingInto{writing, bauerTS}, atmock.New(), writing.FS)
	_, status, b = e.plan(t, "/api/v1/media/trash/plan", fmt.Sprintf(`{"paths":[%q]}`, bauerTS))
	if code, _, _ := errCode(t, b); status != http.StatusConflict || code != "media_in_use" || !strings.Contains(string(b), `"recording"`) {
		t.Fatalf("trashing a recording being written must be refused, got %d %s", status, b)
	}
}

// recordingInto reports a running timer writing into file.
type recordingInto struct {
	*mock.Client
	file string
}

func (b recordingInto) TimerList(ctx context.Context) ([]webcontrol.Timer, error) {
	timers, err := b.Client.TimerList(ctx)
	return append(timers, webcontrol.Timer{Name: "running", State: webcontrol.TimerRunning, Filename: b.file}), err
}

func TestMedia_InUseAtApplyTimeIsRefused(t *testing.T) {
	box := mock.New()
	e := newEnvWith(t, box)
	p, status, b := e.plan(t, "/api/v1/media/rename/plan", fmt.Sprintf(`{"path":%q,"newName":"X"}`, bauerTS))
	if status != http.StatusOK {
		t.Fatalf("plan: %d %s", status, b)
	}
	box.Playing = "1:0:0:0:0:0:0:0:0:0:" + bauerTS // started playing after the preview
	resp, b := e.apply(t, p.Plan.ID)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "media_in_use" {
		t.Fatalf("expected 409 media_in_use at apply, got %d %s", resp.StatusCode, b)
	}
	if !strings.Contains(fmt.Sprint(e.folder(t, "").Movies), "20260608 2048") {
		t.Error("nothing may have been renamed")
	}
}

func TestMedia_StaleRenameIsRejected(t *testing.T) {
	e := newEnv(t)
	a, _, _ := e.plan(t, "/api/v1/media/rename/plan", fmt.Sprintf(`{"path":%q,"newName":"A"}`, bauerTS))
	b, _, _ := e.plan(t, "/api/v1/media/rename/plan", fmt.Sprintf(`{"path":%q,"newName":"B"}`, bauerTS))
	if resp, body := e.apply(t, b.Plan.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply B: %d %s", resp.StatusCode, body)
	}
	resp, body := e.apply(t, a.Plan.ID)
	if code, _, _ := errCode(t, body); resp.StatusCode != http.StatusConflict || code != "plan_stale" {
		t.Fatalf("plan A no longer describes the files: expected plan_stale, got %d %s", resp.StatusCode, body)
	}
}

func TestMedia_TrashRestoreEmpty(t *testing.T) {
	box := mock.New()
	e := newEnvWith(t, box)
	p, status, b := e.plan(t, "/api/v1/media/trash/plan", fmt.Sprintf(`{"paths":[%q]}`, bauerTS))
	if status != http.StatusOK || p.Plan.Action != "media.trash" {
		t.Fatalf("trash plan: %d %s", status, b)
	}
	if resp, b := e.apply(t, p.Plan.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	if len(e.folder(t, "").Movies) != 1 {
		t.Fatal("a trashed recording must disappear from the listing")
	}

	_, b = e.do(t, "GET", "/api/v1/media/trash", "", nil)
	var trash struct {
		Items []struct {
			Path, Title string
			Size        int64
		} `json:"items"`
	}
	json.Unmarshal(b, &trash)
	if len(trash.Items) != 2 {
		t.Fatalf("expected the new and the fixture's trashed recording, got %s", b)
	}

	restore, status, b := e.plan(t, "/api/v1/media/trash/restore/plan", fmt.Sprintf(`{"paths":[%q]}`, bauerTS+".trash"))
	if status != http.StatusOK {
		t.Fatalf("restore plan: %d %s", status, b)
	}
	e.apply(t, restore.Plan.ID)
	if len(e.folder(t, "").Movies) != 2 {
		t.Fatal("the restored recording must be listed again")
	}

	old := mock.Root + "20250101 2015 - ZDF - Alte Doku.ts.trash"
	empty, status, b := e.plan(t, "/api/v1/media/trash/empty/plan", fmt.Sprintf(`{"paths":[%q]}`, old))
	if status != http.StatusOK || len(empty.Plan.Changes) != 2 {
		t.Fatalf("empty plan must list the trashed file and its .ts.meta: %d %s", status, b)
	}
	before := len(box.FS.Paths())
	if resp, b := e.apply(t, empty.Plan.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply empty: %d %s", resp.StatusCode, b)
	}
	if len(box.FS.Paths()) != before-2 {
		t.Errorf("emptying must delete exactly the listed files")
	}
}

func TestMedia_OffTheBoxFileOpsAreUnavailableNotBroken(t *testing.T) {
	e := newEnvWithOptions(t, mock.New(), atmock.New(), nil)
	f := e.folder(t, "")
	if f.FileOps.Available || f.FileOps.Reason != "notOnBox" || len(f.Movies) == 0 {
		t.Fatalf("browsing works, file operations say why they're off: %+v", f)
	}
	_, status, b := e.plan(t, "/api/v1/media/trash/plan", fmt.Sprintf(`{"paths":[%q]}`, bauerTS))
	if code, _, _ := errCode(t, b); status != http.StatusConflict || code != "file_ops_unavailable" {
		t.Fatalf("expected 409 file_ops_unavailable, got %d %s", status, b)
	}
}
