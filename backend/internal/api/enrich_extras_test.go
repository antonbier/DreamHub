package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func TestEnrich_Extras(t *testing.T) {
	e := newEnvWith(t, mock.New())
	get := func(q string) (int, map[string]any) {
		resp, b := e.do(t, "GET", "/api/v1/enrich/extras"+q, "", nil)
		var out map[string]any
		json.Unmarshal(b, &out)
		return resp.StatusCode, out
	}
	if code, _ := get("?kind=person&id=1"); code != http.StatusBadRequest {
		t.Errorf("only films and series: %d", code)
	}
	if _, out := get("?kind=movie&id=603"); out["extras"] != nil {
		t.Errorf("without keys, nothing: %v", out)
	}

	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"tmdb.apiKey":"k-1","omdb.apiKey":"o-1","fanart.apiKey":"f-1"}}`, nil)
	_, out := get("?kind=tv&id=1003")
	x, _ := out["extras"].(map[string]any)
	if x == nil || x["imdbId"] != "tt0001003" || x["rottenTomatoes"] == nil || x["logo"] != "/fanart/tv/71003/hdtvlogo/mock-logo.png" {
		t.Fatalf("ratings by IMDb ID, the logo by TheTVDB's: %v", out)
	}

	resp, b := e.do(t, "GET", "/api/v1/enrich/logo?path="+x["logo"].(string), "", nil)
	if resp.StatusCode != http.StatusOK || http.DetectContentType(b) != "image/png" {
		t.Errorf("logo served: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "GET", "/api/v1/enrich/logo?path=/etc/passwd", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("only fanart.tv paths: %d", resp.StatusCode)
	}
}
