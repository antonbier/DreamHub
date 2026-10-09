package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

const bauer = mock.Root + "20260608 2048 - RTL Television - Bauer sucht Frau International.ts"

type adScanStart struct {
	Items []struct {
		Path     string `json:"path"`
		JobID    int64  `json:"jobId"`
		Existing bool   `json:"existing"`
		Reason   string `json:"reason"`
	} `json:"items"`
}

type adScanAnswer struct {
	ServiceRef string `json:"serviceRef"`
	Result     struct {
		Duration float64 `json:"duration"`
		Blocks   []struct {
			Start     float64 `json:"start"`
			End       float64 `json:"end"`
			Uncertain bool    `json:"uncertain"`
		} `json:"blocks"`
		Probes  int  `json:"probes"`
		Learned bool `json:"learned"`
	} `json:"result"`
}

// waitJob polls GET /api/v1/jobs/{id} until the job has ended.
func (e *env) waitJob(t *testing.T, id int64) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var j struct {
			State string `json:"state"`
			Error string `json:"error"`
		}
		e.json(t, "GET", "/api/v1/jobs/"+itoa(id), "", nil, 200, &j)
		switch j.State {
		case "done", "canceled":
			return j.State
		case "failed":
			t.Fatalf("job %d failed: %s", id, j.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %d didn't finish", id)
	return ""
}

func TestAdScan_FindsBreaksInTheBackground(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(bauer), "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("before any analysis: %d %s", resp.StatusCode, b)
	}

	body, _ := json.Marshal(map[string]any{"paths": []string{bauer, mock.Root + "Oskars Kleid.mkv"}})
	var start adScanStart
	e.json(t, "POST", "/api/v1/media/adscan", string(body), nil, 202, &start)
	if len(start.Items) != 2 || start.Items[0].JobID == 0 || start.Items[1].Reason != "not_a_recording" {
		t.Fatalf("start: %+v", start.Items)
	}
	e.waitJob(t, start.Items[0].JobID)

	var got adScanAnswer
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(bauer), "", nil, 200, &got)
	if got.ServiceRef != "1:0:19:2EE3:441:1:C00000:0:0:0:" || got.Result.Duration != 3600 || len(got.Result.Blocks) != 3 || !got.Result.Learned {
		t.Fatalf("result: %+v", got)
	}
	// The box's movie list reports "//media/…": the same result.
	var viaList adScanAnswer
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape("/"+bauer), "", nil, 200, &viaList)
	if len(viaList.Result.Blocks) != 3 {
		t.Fatalf("by the list's path: %+v", viaList)
	}
	if _, ok, _ := e.store.AdScanLogo(context.Background(), got.ServiceRef); !ok {
		t.Fatal("the channel's logo wasn't kept")
	}

	// Again: the channel's logo is reused, nothing to learn.
	e.json(t, "POST", "/api/v1/media/adscan", `{"paths":["`+bauer+`"]}`, nil, 202, &start)
	e.waitJob(t, start.Items[0].JobID)
	got = adScanAnswer{} // "learned" is left out when false
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(bauer), "", nil, 200, &got)
	if got.Result.Learned || len(got.Result.Blocks) != 3 {
		t.Fatalf("second run: learned %v, %d blocks", got.Result.Learned, len(got.Result.Blocks))
	}
}

func TestAdScan_ViewersMayNotStartIt(t *testing.T) {
	e := newEnv(t)
	_, viewer, _ := e.household(t)
	resp, _ := e.do(t, "POST", "/api/v1/media/adscan", `{"paths":["`+bauer+`"]}`, withProfile(viewer))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a viewer started an analysis: %d", resp.StatusCode)
	}
}

// analyse runs ad detection on one recording and waits for it.
func (e *env) analyse(t *testing.T, p string) adScanAnswer {
	t.Helper()
	var start adScanStart
	e.json(t, "POST", "/api/v1/media/adscan", `{"paths":["`+p+`"]}`, nil, 202, &start)
	e.waitJob(t, start.Items[0].JobID)
	var got adScanAnswer
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(p), "", nil, 200, &got)
	return got
}

func TestAdScan_ResultFollowsRenameTrashAndRestore(t *testing.T) {
	e := newEnv(t)
	e.analyse(t, bauer)

	renamed := mock.Root + "Bauer - Folge 7.ts"
	p, status, b := e.plan(t, "/api/v1/media/rename/plan", `{"path":"`+bauer+`","newName":"Bauer - Folge 7"}`)
	if status != 200 {
		t.Fatalf("rename plan: %d %s", status, b)
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(renamed), "", nil, 200, nil)
	if resp, _ := e.do(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(bauer), "", nil); resp.StatusCode != 404 {
		t.Fatalf("the old path still has a result: %d", resp.StatusCode)
	}

	p, _, _ = e.plan(t, "/api/v1/media/trash/plan", `{"paths":["`+renamed+`"]}`)
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
	p, _, _ = e.plan(t, "/api/v1/media/trash/restore/plan", `{"paths":["`+renamed+`.trash"]}`)
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil, 200, nil)
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(renamed), "", nil, 200, nil)
}

func TestTrim_CutsTheBreaksAndKeepsTheOriginalInTheTrash(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	got := e.analyse(t, bauer)

	type span struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	}
	var cut []span
	for _, b := range got.Result.Blocks {
		cut = append(cut, span{b.Start, b.End})
	}
	blocks, _ := json.Marshal(cut)
	var pr struct {
		Plan struct {
			ID      string `json:"id"`
			Changes []struct {
				Type string `json:"type"`
			} `json:"changes"`
		} `json:"plan"`
		Keep []struct{ Start, End float64 } `json:"keep"`
	}
	e.json(t, "POST", "/api/v1/media/trim/plan", `{"path":"`+bauer+`","blocks":`+string(blocks)+`}`, nil, 200, &pr)
	if len(pr.Keep) != 4 || len(pr.Plan.Changes) != 2 {
		t.Fatalf("plan: keep %+v, changes %+v", pr.Keep, pr.Plan.Changes)
	}
	// Nothing changed yet.
	if content(m, bauer) != "recording" {
		t.Fatal("previewing changed the recording")
	}
	e.json(t, "POST", "/api/v1/plans/"+pr.Plan.ID+"/apply", "", nil, 200, nil)

	var list struct {
		Jobs []struct {
			ID   int64  `json:"id"`
			Kind string `json:"kind"`
		} `json:"jobs"`
	}
	e.json(t, "GET", "/api/v1/jobs", "", nil, 200, &list)
	var trimJob int64
	for _, j := range list.Jobs {
		if j.Kind == "media.trim" {
			trimJob = j.ID
		}
	}
	e.waitJob(t, trimJob)

	base := mock.Root + "20260608 2048 - RTL Television - Bauer sucht Frau International"
	orig := base + " - original"
	if c := content(m, bauer); c != "trimmed: recording" {
		t.Fatalf("the recording's place holds %q", c)
	}
	for _, f := range []string{bauer + ".meta", base + ".eit", base + ".jpg"} {
		if !exists(m, f) {
			t.Errorf("%s wasn't copied to the cut version", f)
		}
	}
	for _, f := range []string{bauer + ".ap", bauer + ".sc", bauer + ".cuts"} {
		if exists(m, f) {
			t.Errorf("%s belongs to the original, not the cut version", f)
		}
	}
	for _, f := range []string{orig + ".ts.trash", orig + ".ts.ap", orig + ".ts.cuts", orig + ".ts.meta"} {
		if !exists(m, f) {
			t.Errorf("%s is missing: the original must be kept", f)
		}
	}

	var after adScanAnswer
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(bauer), "", nil, 200, &after)
	if len(after.Result.Blocks) != 0 || after.Result.Duration >= got.Result.Duration {
		t.Fatalf("after cutting: %+v", after.Result)
	}
	// The original's analysis went with it into the trash.
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(orig+".ts.trash"), "", nil, 200, nil)
}

func TestTrim_NeedsAnAnalysisAndSaneBlocks(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "POST", "/api/v1/media/trim/plan", `{"path":"`+bauer+`","blocks":[{"start":10,"end":20}]}`, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("without analysis: %d %s", resp.StatusCode, b)
	}
	e.analyse(t, bauer)
	resp, b = e.do(t, "POST", "/api/v1/media/trim/plan", `{"path":"`+bauer+`","blocks":[{"start":0,"end":99999}]}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a block past the end: %d %s", resp.StatusCode, b)
	}
}

func content(m *mock.Client, p string) string {
	b, _ := m.FS.ReadFile(p)
	return string(b)
}

func exists(m *mock.Client, p string) bool {
	_, err := m.FS.Lstat(p)
	return err == nil
}
