package api_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/adscan"
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

// syntheticAP builds a .ts.ap with one access point per second, covering
// seconds; offsets are arbitrary, only the PTS (90 kHz) matters here.
func syntheticAP(seconds int) []byte {
	out := make([]byte, 16*seconds)
	for i := 0; i < seconds; i++ {
		binary.BigEndian.PutUint64(out[16*i:], uint64(i*188000))
		binary.BigEndian.PutUint64(out[16*i+8:], uint64(i)*90000)
	}
	return out
}

func TestMark_WritesBreaksIntoTheCutList(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	// The fixture's .ts.ap and .ts.cuts are placeholder text, not real
	// sidecars (recordingFixtures) — a real recording always has a real
	// .ts.ap, so this stands in for one; an empty .ts.cuts for "no cuts
	// yet", which is the common case (ROADMAP.md, phase 0's box check).
	if err := m.FS.WriteFile(bauer+".ap", syntheticAP(3600)); err != nil {
		t.Fatal(err)
	}
	if err := m.FS.WriteFile(bauer+".cuts", nil); err != nil {
		t.Fatal(err)
	}
	got := e.analyse(t, bauer)
	if len(got.Result.Blocks) != 3 {
		t.Fatalf("fixture changed: %d blocks", len(got.Result.Blocks))
	}

	type span struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	}
	var blocks []span
	for _, b := range got.Result.Blocks {
		blocks = append(blocks, span{b.Start, b.End})
	}
	body, _ := json.Marshal(map[string]any{"path": bauer, "blocks": blocks})
	var pr struct {
		Plan struct {
			ID      string `json:"id"`
			Changes []struct {
				Type string `json:"type"`
			} `json:"changes"`
		} `json:"plan"`
	}
	e.json(t, "POST", "/api/v1/media/adscan/mark/plan", string(body), nil, 200, &pr)
	if len(pr.Plan.Changes) != 1 || pr.Plan.Changes[0].Type != "recording.mark" {
		t.Fatalf("plan: %+v", pr.Plan.Changes)
	}
	// Nothing changed yet.
	if raw, _ := m.FS.ReadFile(bauer + ".cuts"); len(raw) != 0 {
		t.Fatal("previewing changed the cut list")
	}
	e.json(t, "POST", "/api/v1/plans/"+pr.Plan.ID+"/apply", "", nil, 200, nil)

	raw, err := m.FS.ReadFile(bauer + ".cuts")
	if err != nil {
		t.Fatal(err)
	}
	cuts, err := adscan.ParseCuts(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cuts) != 2*len(got.Result.Blocks) {
		t.Fatalf("cuts: %+v", cuts)
	}
	for i, b := range got.Result.Blocks {
		out, in := cuts[2*i], cuts[2*i+1]
		if out.Type != adscan.CutOut || in.Type != adscan.CutIn {
			t.Fatalf("block %d: types %d, %d", i, out.Type, in.Type)
		}
		// One access point per second: the PTS is the floor of the
		// block's boundary, times 90 kHz.
		if want := uint64(int64(b.Start)) * 90000; out.PTS != want {
			t.Errorf("block %d out: PTS %d, want %d", i, out.PTS, want)
		}
		if want := uint64(int64(b.End)) * 90000; in.PTS != want {
			t.Errorf("block %d in: PTS %d, want %d", i, in.PTS, want)
		}
	}

	var after adScanAnswer
	var afterFull struct {
		Result struct {
			Marked bool `json:"marked"`
		} `json:"result"`
	}
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(bauer), "", nil, 200, &after)
	e.json(t, "GET", "/api/v1/media/adscan?path="+url.QueryEscape(bauer), "", nil, 200, &afterFull)
	if !afterFull.Result.Marked {
		t.Fatal("the result wasn't flagged as marked")
	}
}

func TestMark_KeepsExistingCutsAndUnitesOverlaps(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	if err := m.FS.WriteFile(bauer+".ap", syntheticAP(3600)); err != nil {
		t.Fatal(err)
	}
	got := e.analyse(t, bauer)
	if len(got.Result.Blocks) == 0 {
		t.Fatal("fixture changed: no blocks")
	}
	// A resume position, and a hand-made cut starting 50 s before the
	// first ad block and ending 50 s into it (every synthetic block is
	// at least 180 s wide, so this always overlaps).
	blockStart := uint64(int64(got.Result.Blocks[0].Start)) * 90000
	existing := []adscan.Cut{
		{PTS: blockStart - 50*90000, Type: adscan.CutOut}, {PTS: blockStart + 50*90000, Type: adscan.CutIn},
		{PTS: 3000 * 90000, Type: adscan.CutLast},
	}
	if err := m.FS.WriteFile(bauer+".cuts", adscan.EncodeCuts(existing)); err != nil {
		t.Fatal(err)
	}

	type span struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	}
	var blocks []span
	for _, b := range got.Result.Blocks {
		blocks = append(blocks, span{b.Start, b.End})
	}
	body, _ := json.Marshal(map[string]any{"path": bauer, "blocks": blocks})
	var pr struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	e.json(t, "POST", "/api/v1/media/adscan/mark/plan", string(body), nil, 200, &pr)
	e.json(t, "POST", "/api/v1/plans/"+pr.Plan.ID+"/apply", "", nil, 200, nil)

	raw, _ := m.FS.ReadFile(bauer + ".cuts")
	cuts, err := adscan.ParseCuts(raw)
	if err != nil {
		t.Fatal(err)
	}
	var lastKept, markCount int
	for _, c := range cuts {
		if c.Type == adscan.CutLast {
			lastKept++
		}
		if c.Type == adscan.CutOut {
			markCount++
		}
	}
	if lastKept != 1 {
		t.Fatalf("the resume position wasn't kept: %+v", cuts)
	}
	// The hand-made cut united with the first ad block into one region,
	// so there are fewer OUT entries than ad blocks plus one.
	if markCount != len(got.Result.Blocks) {
		t.Fatalf("regions: %d OUT entries for %d blocks: %+v", markCount, len(got.Result.Blocks), cuts)
	}
	if cuts[0].PTS > blockStart-50*90000 {
		t.Fatalf("the united region should start at or before the hand-made cut: %+v", cuts[0])
	}
}

// trimAll cuts every found break out of p and waits for the cut.
func (e *env) trimAll(t *testing.T, p string) {
	t.Helper()
	got := e.analyse(t, p)
	var cut []map[string]float64
	for _, b := range got.Result.Blocks {
		cut = append(cut, map[string]float64{"start": b.Start, "end": b.End})
	}
	if len(cut) == 0 { // a cut version: invent a break to cut again
		cut = append(cut, map[string]float64{"start": 100, "end": 200})
	}
	body, _ := json.Marshal(map[string]any{"path": p, "blocks": cut})
	var pr struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	e.json(t, "POST", "/api/v1/media/trim/plan", string(body), nil, 200, &pr)
	e.json(t, "POST", "/api/v1/plans/"+pr.Plan.ID+"/apply", "", nil, 200, nil)
	var list struct {
		Jobs []struct {
			ID    int64  `json:"id"`
			Kind  string `json:"kind"`
			State string `json:"state"`
		} `json:"jobs"`
	}
	e.json(t, "GET", "/api/v1/jobs", "", nil, 200, &list)
	for _, j := range list.Jobs {
		if j.Kind == "media.trim" && j.State != "done" {
			e.waitJob(t, j.ID)
		}
	}
}

func TestTrim_TwiceKeepsBothOriginals(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	e.trimAll(t, bauer)
	e.trimAll(t, bauer)
	base := mock.Root + "20260608 2048 - RTL Television - Bauer sucht Frau International"
	for _, f := range []string{base + " - original.ts.trash", base + " - original 2.ts.trash", base + " - original 2.ts.meta"} {
		if !exists(m, f) {
			t.Errorf("%s is missing", f)
		}
	}
	if c := content(m, bauer); c != "trimmed: trimmed: recording" {
		t.Fatalf("after two cuts: %q", c)
	}
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
