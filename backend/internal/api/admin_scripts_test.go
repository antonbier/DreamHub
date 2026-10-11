package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

type scriptList struct {
	Scripts []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"scripts"`
}

func (e *env) uploadScript(t *testing.T, name, content string) (int, int64) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "content": content})
	resp, b := e.do(t, "POST", "/api/v1/admin/scripts", string(body), nil)
	var v struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(b, &v)
	return resp.StatusCode, v.ID
}

type runPlan struct {
	Plan struct {
		ID      string `json:"id"`
		Changes []struct {
			Type string `json:"type"`
		} `json:"changes"`
	} `json:"plan"`
}

func (e *env) planRunScript(t *testing.T, id int64) (int, runPlan) {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/admin/scripts/"+strconv.FormatInt(id, 10)+"/run/plan", "", nil)
	var p runPlan
	json.Unmarshal(b, &p)
	return resp.StatusCode, p
}

func TestAdminScripts_UploadListAndDelete(t *testing.T) {
	e := newEnv(t)
	code, id := e.uploadScript(t, "cleanup.sh", "#!/bin/sh\necho hi\n")
	if code != http.StatusCreated || id == 0 {
		t.Fatalf("upload: %d, id %d", code, id)
	}

	resp, b := e.do(t, "GET", "/api/v1/admin/scripts", "", nil)
	var list scriptList
	json.Unmarshal(b, &list)
	if resp.StatusCode != http.StatusOK || len(list.Scripts) != 1 || list.Scripts[0].Name != "cleanup.sh" {
		t.Fatalf("list: %d %+v", resp.StatusCode, list)
	}

	resp, _ = e.do(t, "DELETE", "/api/v1/admin/scripts/"+strconv.FormatInt(id, 10), "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	_, b = e.do(t, "GET", "/api/v1/admin/scripts", "", nil)
	json.Unmarshal(b, &list)
	if len(list.Scripts) != 0 {
		t.Fatalf("expected no scripts left, got %+v", list)
	}
}

func TestAdminScripts_RejectsAnOversizedUpload(t *testing.T) {
	e := newEnv(t)
	big := make([]byte, 300*1024)
	for i := range big {
		big[i] = 'a'
	}
	code, _ := e.uploadScript(t, "huge.sh", string(big))
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an oversized script, got %d", code)
	}
}

func TestAdminScripts_RunGoesThroughPreviewAndIsLogged(t *testing.T) {
	e := newEnv(t)
	_, id := e.uploadScript(t, "hello.sh", "#!/bin/sh\necho hello\n")
	e.scripts.Output, e.scripts.ExitCode = "hello\n", 0

	code, p := e.planRunScript(t, id)
	if code != http.StatusOK || len(p.Plan.Changes) != 1 || p.Plan.Changes[0].Type != "script.run" {
		t.Fatalf("plan: %d %+v", code, p)
	}
	if len(e.scripts.Ran()) != 0 {
		t.Fatal("previewing a run must not execute the script")
	}
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	if ran := e.scripts.Ran(); len(ran) != 1 || ran[0] != "#!/bin/sh\necho hello\n" {
		t.Fatalf("ran = %v", ran)
	}

	resp, b := e.do(t, "GET", "/api/v1/admin/scripts/"+strconv.FormatInt(id, 10)+"/runs", "", nil)
	var runs struct {
		Runs []struct {
			ExitCode int    `json:"exitCode"`
			Output   string `json:"output"`
		} `json:"runs"`
	}
	json.Unmarshal(b, &runs)
	if resp.StatusCode != http.StatusOK || len(runs.Runs) != 1 || runs.Runs[0].ExitCode != 0 || runs.Runs[0].Output != "hello\n" {
		t.Fatalf("runs: %d %+v", resp.StatusCode, runs)
	}
}

func TestAdminScripts_StalePreviewIsRefusedAfterDelete(t *testing.T) {
	e := newEnv(t)
	_, id := e.uploadScript(t, "a.sh", "echo a")
	_, p := e.planRunScript(t, id)
	// The script is deleted between preview and apply.
	e.do(t, "DELETE", "/api/v1/admin/scripts/"+strconv.FormatInt(id, 10), "", nil)

	resp, _ := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("a preview made before the script was deleted must not apply")
	}
	if len(e.scripts.Ran()) != 0 {
		t.Error("a stale plan must not run anything")
	}
}

func TestAdminScripts_ReadOnlyRunsNothing(t *testing.T) {
	e := newEnvWithReadOnlyFiles(t, mock.New())
	_, id := e.uploadScript(t, "a.sh", "echo a")
	_, p := e.planRunScript(t, id)
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "writes_disabled" {
		t.Fatalf("expected 409 writes_disabled, got %d %s", resp.StatusCode, b)
	}
	if len(e.scripts.Ran()) != 0 {
		t.Errorf("ran = %v", e.scripts.Ran())
	}
}
