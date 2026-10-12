package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type searchResults struct {
	Results []struct {
		HubID string `json:"hubId"`
		Title string `json:"title"`
		Path  string `json:"path"`
	} `json:"results"`
}

func (e *env) search(t *testing.T, q string) searchResults {
	t.Helper()
	resp, b := e.do(t, "GET", "/api/v1/search?q="+q, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search %q: %d %s", q, resp.StatusCode, b)
	}
	var r searchResults
	json.Unmarshal(b, &r)
	return r
}

func (r searchResults) from(hubID string) []string {
	var out []string
	for _, x := range r.Results {
		if x.HubID == hubID {
			out = append(out, x.Title+" → "+x.Path)
		}
	}
	return out
}

// The hubs that joined global search with the rest of the hub contract.
func TestSearch_DiscoverySystemAdmin(t *testing.T) {
	e := newEnv(t)

	// DeviceHub's admin half: services by name or description.
	if got := e.search(t, "samba").from("device"); len(got) != 2 || !strings.HasPrefix(got[0], "smb → /device?tab=admin") {
		t.Errorf("admin results: %v", got)
	}

	// DeviceHub's system half: the change log, opened already filtered.
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"ai.openai.apiKey":"sk-test-1234"}}`, nil)
	if got := e.search(t, "openai").from("device"); len(got) == 0 || !strings.HasSuffix(got[0], "/device?changes=openai") {
		t.Errorf("system results: %v", got)
	}

	// DiscoveryHub: the day's saved picks.
	bRef := `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`
	resp, b := e.do(t, "POST", "/api/v1/discovery/recommendations", `{"bouquet":`+strconv.Quote(bRef)+`}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("generate: %d %s", resp.StatusCode, b)
	}
	var picks struct {
		Recommendations []struct {
			Title string `json:"title"`
		} `json:"recommendations"`
	}
	json.Unmarshal(b, &picks)
	if len(picks.Recommendations) == 0 {
		t.Fatal("no picks to search for")
	}
	title := picks.Recommendations[0].Title
	word := strings.Fields(title)[0]
	if got := e.search(t, word).from("discovery"); len(got) == 0 || !strings.Contains(strings.Join(got, "|"), title) {
		t.Errorf("discovery results for %q: %v", word, got)
	}
}

type viewResult struct {
	Items []struct {
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		Warn     bool   `json:"warn"`
		Path     string `json:"path"`
	} `json:"items"`
	Total int `json:"total"`
}

func (e *env) view(t *testing.T, body string) viewResult {
	t.Helper()
	resp, b := e.do(t, "POST", "/api/v1/dashboard/view", body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("view %s: %d %s", body, resp.StatusCode, b)
	}
	var v viewResult
	json.Unmarshal(b, &v)
	return v
}

func TestViews_DiscoveryPicksAndRadioNow(t *testing.T) {
	e := newEnv(t)
	// DiscoveryHub is unavailable (and offers no source) without a key.
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"ai.openai.apiKey":"sk-test-1234"}}`, nil)

	// Both new sources are offered.
	_, b := e.do(t, "GET", "/api/v1/dashboard/sources", "", nil)
	for _, id := range []string{"discovery.picks", "radio.now"} {
		if !strings.Contains(string(b), `"`+id+`"`) {
			t.Errorf("source %s not offered: %s", id, b)
		}
	}

	// Radio at the test clock (20:07 local): the mock's favourites have
	// programmes; Klassik Radio, without EPG, isn't listed.
	radio := e.view(t, `{"source":"radio.now","query":{},"timeZone":"Europe/Rome"}`)
	if radio.Total == 0 {
		t.Fatal("expected programmes on the radio")
	}
	for _, it := range radio.Items {
		if it.Subtitle == "Klassik Radio" || !strings.HasPrefix(it.Path, "/radio?") {
			t.Errorf("unexpected item %+v", it)
		}
	}

	// Picks: none before generating, then each marked as without a timer.
	if v := e.view(t, `{"source":"discovery.picks","query":{},"timeZone":"Europe/Rome"}`); v.Total != 0 {
		t.Errorf("no picks yet, got %+v", v)
	}
	bRef := `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`
	e.do(t, "POST", "/api/v1/discovery/recommendations", `{"bouquet":`+strconv.Quote(bRef)+`}`, nil)
	v := e.view(t, `{"source":"discovery.picks","query":{"warnOnly":true},"timeZone":"Europe/Rome"}`)
	if v.Total == 0 {
		t.Fatalf("expected open picks, got %+v", v)
	}
}

// GET /api/v1/hubs tells which events each hub's area emits.
func TestHubs_DeclareTheirEvents(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/hubs", "", nil)
	var body struct {
		Hubs []struct {
			ID    string   `json:"id"`
			Emits []string `json:"emits"`
		} `json:"hubs"`
	}
	json.Unmarshal(b, &body)
	emits := map[string][]string{}
	for _, h := range body.Hubs {
		emits[h.ID] = h.Emits
	}
	if strings.Join(emits["device"], ",") != "service.failed,service.recovered,package.upgradable" || len(emits["timer"]) != 3 || emits["control"] == nil {
		t.Errorf("emits: %v", emits)
	}
}
