package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

type selfTestReport struct {
	Report *struct {
		Checks []struct {
			ID     string         `json:"id"`
			Status string         `json:"status"`
			Code   string         `json:"code"`
			Params map[string]any `json:"params"`
		} `json:"checks"`
		Captured []string `json:"captured"`
	} `json:"report"`
}

func TestSelfTest_RunsAgainstTheMockAndKeepsTheReport(t *testing.T) {
	e := newEnvWith(t, mock.New())

	// Nothing has run yet.
	_, b := e.do(t, "GET", "/api/v1/system/selftest", "", nil)
	var none selfTestReport
	json.Unmarshal(b, &none)
	if none.Report != nil {
		t.Fatalf("expected no report yet, got %s", b)
	}

	resp, b := e.do(t, "POST", "/api/v1/system/selftest", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var rep selfTestReport
	json.Unmarshal(b, &rep)
	got := map[string]string{}
	codes := map[string]string{}
	for _, c := range rep.Report.Checks {
		got[c.ID] = c.Status
		codes[c.ID] = c.Code
	}
	for id, want := range map[string]string{
		"box": "ok", "writes": "ok", "tvBouquets": "ok", "tvEpg": "ok", "current": "ok",
		"radioBouquets": "ok", "radioStations": "ok", "timers": "ok", "recordings": "ok",
		"serienrecorder": "ok", "autotimer": "ok", "services": "warn", // nmbd is down in the mock
	} {
		if got[id] != want {
			t.Errorf("%s: %q (%s), want %q", id, got[id], codes[id], want)
		}
	}
	if codes["services"] != "servicesDown" {
		t.Errorf("services code %q", codes["services"])
	}
	// The mock client can't give raw answers: nothing captured.
	if len(rep.Report.Captured) != 0 {
		t.Errorf("captured %v in mock mode", rep.Report.Captured)
	}

	_, b = e.do(t, "GET", "/api/v1/system/selftest", "", nil)
	var again selfTestReport
	json.Unmarshal(b, &again)
	if again.Report == nil || len(again.Report.Checks) != len(rep.Report.Checks) {
		t.Fatalf("last report not kept: %s", b)
	}

	resp, b = e.do(t, "GET", "/api/v1/system/selftest/download", "", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("download: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil || len(zr.File) == 0 || zr.File[len(zr.File)-1].Name != "report.json" {
		t.Fatalf("zip: %v %+v", err, zr)
	}
}
