package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

func TestMedia_ReadOnlyRunPreviewsButNeverTouchesFiles(t *testing.T) {
	box := mock.New()
	e := newEnvWithReadOnlyFiles(t, box)
	before := box.FS.Paths()
	p, status, b := e.plan(t, "/api/v1/media/trash/plan", fmt.Sprintf(`{"paths":[%q]}`, bauerTS))
	if status != http.StatusOK {
		t.Fatalf("previews still work read-only: %d %s", status, b)
	}
	resp, b := e.apply(t, p.Plan.ID)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "writes_disabled" {
		t.Fatalf("expected 409 writes_disabled, got %d %s", resp.StatusCode, b)
	}
	if after := box.FS.Paths(); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Error("a read-only run must not change any file")
	}
}

type suggestResp struct {
	Suggestions []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Kind   string `json:"kind"`
	} `json:"suggestions"`
}

func (e *env) suggest(t *testing.T, path, source string) (int, suggestResp, []byte) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/media/rename/suggest", fmt.Sprintf(`{"path":%q,"source":%q}`, path, source), nil)
	var s suggestResp
	json.Unmarshal(b, &s)
	return resp.StatusCode, s, b
}

const tatortTS = mock.Root + "Tatort/20260920 2015 - Das Erste HD - Tatort_ Borowski und das Haus am Meer.ts"

func TestMedia_NameSuggestions(t *testing.T) {
	e := newEnv(t)

	// Metadata: always available, built from .ts.meta; colons made SMB-safe.
	status, s, b := e.suggest(t, tatortTS, "metadata")
	if status != http.StatusOK || len(s.Suggestions) != 1 || s.Suggestions[0].Name != "Tatort - Borowski und das Haus am Meer" {
		t.Fatalf("metadata suggestion: %d %s", status, b)
	}

	// TMDB and AI need their keys first — a clear code, not a failure.
	for source, code := range map[string]string{"tmdb": "tmdb_not_configured", "ai": "ai_not_configured"} {
		status, _, b := e.suggest(t, tatortTS, source)
		if got, _, _ := errCode(t, b); status != http.StatusConflict || got != code {
			t.Errorf("%s without a key: want 409 %s, got %d %s", source, code, status, b)
		}
	}
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"tmdb.apiKey":"k","ai.openai.apiKey":"sk-test","ai.dailyLimit":1}}`, nil)

	status, s, b = e.suggest(t, tatortTS, "tmdb")
	if status != http.StatusOK || len(s.Suggestions) != 2 || s.Suggestions[1].Name != "Tatort - Borowski und das Haus am Meer (2026)" || s.Suggestions[1].Kind != "movie" {
		t.Fatalf("tmdb suggestions: %d %s", status, b)
	}

	status, s, b = e.suggest(t, tatortTS, "ai")
	if status != http.StatusOK || len(s.Suggestions) != 1 || s.Suggestions[0].Source != "ai" {
		t.Fatalf("ai suggestion: %d %s", status, b)
	}
	// The daily budget (1) is used up now.
	status, _, b = e.suggest(t, tatortTS, "ai")
	if got, _, _ := errCode(t, b); status != http.StatusTooManyRequests || got != "ai_budget_exceeded" {
		t.Fatalf("expected ai_budget_exceeded, got %d %s", status, b)
	}

	// A suggestion changes nothing on its own.
	if !strings.Contains(fmt.Sprint(e.folder(t, mock.Root+"Tatort/").Movies), "Tatort_ Borowski") {
		t.Error("suggesting must not rename anything")
	}
}

func TestMedia_RenameCanAlsoChangeTheTitle(t *testing.T) {
	box := mock.New()
	e := newEnvWith(t, box)

	// File name and title together: every file, plus the title change.
	p, status, b := e.plan(t, "/api/v1/media/rename/plan", fmt.Sprintf(`{"path":%q,"newName":"Bauer sucht Frau - Folge 7","title":"Bauer sucht Frau International – Folge 7"}`, "/"+bauerTS))
	if status != http.StatusOK || len(p.Plan.Changes) != 8 || !strings.Contains(string(b), `"meta.title"`) {
		t.Fatalf("plan: %d %s", status, b)
	}
	if resp, b := e.apply(t, p.Plan.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	newTS := mock.Root + "Bauer sucht Frau - Folge 7.ts"
	meta, err := box.FS.ReadFile(newTS + ".meta")
	lines := strings.Split(string(meta), "\n")
	if err != nil || len(lines) < 4 || lines[1] != "Bauer sucht Frau International – Folge 7" || lines[0] != "1:0:19:2EE3:441:1:C00000:0:0:0:" || lines[3] != "1780944521" {
		t.Fatalf("only line 2 may change: %q %v", meta, err)
	}

	// The title alone, file name unchanged.
	p, status, b = e.plan(t, "/api/v1/media/rename/plan", fmt.Sprintf(`{"path":%q,"newName":"Bauer sucht Frau - Folge 7","title":"Nur der Titel"}`, newTS))
	if status != http.StatusOK || len(p.Plan.Changes) != 1 {
		t.Fatalf("title-only plan: %d %s", status, b)
	}
	e.apply(t, p.Plan.ID)
	if got := string(must(box.FS.ReadFile(newTS + ".meta"))); !strings.Contains(got, "\nNur der Titel\n") {
		t.Errorf("title-only change not applied: %q", got)
	}

	for _, bad := range []string{`"title":"zwei\nZeilen"`, `"title":"  "`} {
		_, status, b := e.plan(t, "/api/v1/media/rename/plan", fmt.Sprintf(`{"path":%q,"newName":"X",%s}`, newTS, bad))
		if code, _, _ := errCode(t, b); status != http.StatusBadRequest || code != "invalid_title" {
			t.Errorf("%s: expected 400 invalid_title, got %d %s", bad, status, b)
		}
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func TestMedia_CoverArtAlreadyOnDisk(t *testing.T) {
	e := newEnv(t)
	tatortDir := mock.Root + "Tatort/"
	f := e.folder(t, tatortDir)
	_, b := e.do(t, "GET", "/api/v1/media/movies?dir="+url.QueryEscape(tatortDir), "", nil)
	if len(f.Movies) != 1 || !strings.Contains(string(b), `"cover":true`) || strings.Contains(string(b), `"backdrop":true`) {
		t.Fatalf("the Tatort recording has a cover, no backdrop: %s", b)
	}
	ts := f.Movies[0].Filename
	resp, img := e.do(t, "GET", "/api/v1/media/cover?path="+url.QueryEscape(ts), "", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" || len(img) == 0 {
		t.Fatalf("cover: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, q := range []string{
		"path=" + url.QueryEscape(ts) + "&kind=backdrop", // none there
		"path=" + url.QueryEscape("/"+bauerTS),           // no cover
		"path=" + url.QueryEscape("/etc/passwd"),
		"path=" + url.QueryEscape(mock.Root+"../../etc/x.ts"),
	} {
		if resp, _ := e.do(t, "GET", "/api/v1/media/cover?"+q, "", nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", q, resp.StatusCode)
		}
	}
}

func TestMedia_BatchRenameByScheme(t *testing.T) {
	box := mock.New()
	// Two episodes and a film, as SerienRecorder/Enigma2 name them.
	put := func(base, title, desc string) string {
		box.FS.Put(mock.Root+base+".ts", "recording")
		box.FS.Put(mock.Root+base+".ts.meta", "1:0:19:2EE3:441:1:C00000:0:0:0:\n"+title+"\n"+desc+"\n1696532400\n")
		box.FS.Put(mock.Root+base+".eit", "eit")
		return "/" + mock.Root + base + ".ts"
	}
	e1 := put("20231005 2015 - RTL - Bones", "Bones", "S03E07 - Der Mann im Keller")
	e2 := put("20231012 2015 - RTL - Bones", "Bones", "S03E08")
	film := put("20231020 2015 - ARD - Tatort", "Tatort", "")
	e := newEnvWith(t, box)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"media.naming.episode":"{title} {season}x{episode:02}[ - {episodeTitle}]"}}`, nil)

	body := fmt.Sprintf(`{"paths":[%q,%q,%q,"/etc/passwd"]}`, e1, e2, film)
	resp, b := e.do(t, "POST", "/api/v1/media/rename/batch/plan", body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d %s", resp.StatusCode, b)
	}
	var out struct {
		Plan *struct {
			ID      string `json:"id"`
			Changes []any  `json:"changes"`
		} `json:"plan"`
		Items []struct {
			After  string `json:"after"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"items"`
	}
	json.Unmarshal(b, &out)
	if out.Plan == nil || len(out.Plan.Changes) != 3 || len(out.Items) != 4 {
		t.Fatalf("expected 3 renames and the outside path skipped: %s", b)
	}
	if out.Items[0].After != "Bones 3x07 - Der Mann im Keller" || out.Items[1].After != "Bones 3x08" || out.Items[2].After != "Tatort" {
		t.Errorf("names by the scheme: %+v", out.Items)
	}
	if out.Items[3].Status != "skipped" || out.Items[3].Reason != "outside_folders" {
		t.Errorf("a path outside the recording folders is skipped, not fatal: %+v", out.Items[3])
	}
	if resp, b := e.apply(t, out.Plan.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	for _, f := range []string{"Bones 3x07 - Der Mann im Keller.ts", "Bones 3x07 - Der Mann im Keller.eit", "Bones 3x08.ts.meta", "Tatort.ts"} {
		if _, err := box.FS.Lstat(mock.Root + f); err != nil {
			t.Errorf("%s missing after apply", f)
		}
	}

	// Running it again: everything is already named by the scheme.
	body = fmt.Sprintf(`{"paths":[%q]}`, "/"+mock.Root+"Bones 3x08.ts")
	_, b = e.do(t, "POST", "/api/v1/media/rename/batch/plan", body, nil)
	if strings.Contains(string(b), `"plan"`) || !strings.Contains(string(b), `"unchanged"`) {
		t.Errorf("nothing to rename, no plan: %s", b)
	}

	// TMDB completes a film's year on an exact title match only.
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"tmdb.apiKey":"k-1234567890"}}`, nil)
	body = fmt.Sprintf(`{"paths":[%q],"source":"tmdb"}`, "/"+bauerTS)
	_, b = e.do(t, "POST", "/api/v1/media/rename/batch/plan", body, nil)
	if !strings.Contains(string(b), `"after":"Bauer sucht Frau International"`) {
		t.Errorf("a TV show found by TMDB gets no film year: %s", b)
	}

	// Two recordings that would end up with the same name: only the first.
	d1 := put("A", "Doppelt", "")
	d2 := put("B", "Doppelt", "")
	body = fmt.Sprintf(`{"paths":[%q,%q]}`, d1, d2)
	_, b = e.do(t, "POST", "/api/v1/media/rename/batch/plan", body, nil)
	if !strings.Contains(string(b), `"duplicate_target"`) {
		t.Errorf("a second recording aiming at the same name is skipped: %s", b)
	}

	// A broken scheme is refused when saving.
	resp, b = e.do(t, "PATCH", "/api/v1/settings", `{"values":{"media.naming.movie":"{year}"}}`, nil)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "template_no_title") {
		t.Errorf("a scheme without {title} must be refused: %d %s", resp.StatusCode, b)
	}
}

func TestMedia_BatchRenameIntoFolders(t *testing.T) {
	box := mock.New()
	put := func(base, title, desc string) string {
		box.FS.Put(mock.Root+base+".ts", "recording")
		box.FS.Put(mock.Root+base+".ts.meta", "1:0:19:2EE3:441:1:C00000:0:0:0:\n"+title+"\n"+desc+"\n1696532400\n")
		box.FS.Put(mock.Root+base+".jpg", "cover")
		return "/" + mock.Root + base + ".ts"
	}
	e1 := put("20250429 2300 - SAT_1 - FBI_ Most Wanted - S05E07", "FBI: Most Wanted - S05E07 - Gefangenenübergabe", "S05E07 - Gefangenenübergabe")
	film := put("20231020 2015 - ARD - Heat", "Heat", "")
	e := newEnvWith(t, box)
	resp, b := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"media.naming.folderEpisode":"{title}/Staffel {season:02}","media.naming.folderMovie":"Filme"}}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings: %d %s", resp.StatusCode, b)
	}
	if resp, b := e.do(t, "PATCH", "/api/v1/settings", `{"values":{"media.naming.folderMovie":"../raus"}}`, nil); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "template_folder") {
		t.Errorf("a folder scheme climbing out must be refused: %d %s", resp.StatusCode, b)
	}

	// Without "folders", nothing moves.
	_, b = e.do(t, "POST", "/api/v1/media/rename/batch/plan", fmt.Sprintf(`{"paths":[%q]}`, film), nil)
	if strings.Contains(string(b), `"folder"`) {
		t.Errorf("no folder unless asked: %s", b)
	}

	_, b = e.do(t, "POST", "/api/v1/media/rename/batch/plan", fmt.Sprintf(`{"paths":[%q,%q],"folders":true}`, e1, film), nil)
	var out struct {
		Plan *struct {
			ID string `json:"id"`
		} `json:"plan"`
		Folders []string `json:"folders"`
		Items   []struct {
			After  string `json:"after"`
			Folder string `json:"folder"`
		} `json:"items"`
	}
	json.Unmarshal(b, &out)
	if out.Plan == nil || out.Items[0].Folder != "FBI - Most Wanted/Staffel 05" || out.Items[1].Folder != "Filme" || len(out.Folders) != 3 {
		t.Fatalf("folders by scheme, the new ones listed: %s", b)
	}
	if resp, b := e.apply(t, out.Plan.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	for _, f := range []string{
		"FBI - Most Wanted/Staffel 05/FBI - Most Wanted - S05E07 - Gefangenenübergabe.ts",
		"FBI - Most Wanted/Staffel 05/FBI - Most Wanted - S05E07 - Gefangenenübergabe.jpg",
		"Filme/Heat.ts.meta",
	} {
		if _, err := box.FS.Lstat(mock.Root + f); err != nil {
			t.Errorf("%s missing after apply", f)
		}
	}
	if _, err := box.FS.Lstat(strings.TrimPrefix(e1, "/")); err == nil {
		t.Error("the old file must be gone")
	}
}
