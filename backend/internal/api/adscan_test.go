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
