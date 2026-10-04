package hubs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// A box answering with the captured fixtures, and a radio list in the
// shape getservices answers.
func fixtureBox(t *testing.T) *httptest.Server {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("../../testdata/fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	radioRoot := `<e2servicelist><e2service><e2servicereference>1:7:2:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.radio" ORDER BY bouquet</e2servicereference><e2servicename>Favourites (Radio)</e2servicename></e2service></e2servicelist>`
	radioList := `<e2servicelist><e2service><e2servicereference>1:0:2:6F37:44D:1:C00000:0:0:0:</e2servicereference><e2servicename>BAYERN 3</e2servicename></e2service>
<e2service><e2servicereference>4097:0:2:0:0:0:0:0:0:0:http%3a//stream.example/radio.mp3:Web Radio</e2servicereference><e2servicename>Web Radio</e2servicename></e2service>
<e2service><e2servicereference>1:0:19:2EE3:441:1:C00000:0:0:0:</e2servicereference><e2servicename>RTL Television</e2servicename></e2service></e2servicelist>`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sRef := r.URL.Query().Get("sRef")
		switch {
		case r.URL.Path == "/web/getservices" && sRef == webcontrol.RadioRoot:
			w.Write([]byte(radioRoot))
		case r.URL.Path == "/web/getservices" && strings.Contains(sRef, "radio"):
			w.Write([]byte(radioList))
		case r.URL.Path == "/web/getservices":
			w.Write(read("getservices.xml"))
		case r.URL.Path == "/web/epgnownext":
			w.Write(read("epgnownext.xml"))
		default:
			b, err := os.ReadFile(filepath.Join("../../testdata/fixtures", strings.TrimPrefix(r.URL.Path, "/web/")+".xml"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Write(b)
		}
	}))
}

func TestSelfTest_CapturesTheBoxAnswers(t *testing.T) {
	srv := fixtureBox(t)
	defer srv.Close()
	dir := filepath.Join(t.TempDir(), "selftest")
	st := &SelfTest{WC: webcontrol.NewHTTPClient(srv.URL), Dir: dir, PiconDirs: []string{t.TempDir()}}
	rep, err := st.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]SelfTestCheck{}
	for _, c := range rep.Checks {
		status[c.ID] = c
	}
	if c := status["box"]; c.Status != CheckOK {
		t.Errorf("box: %+v", c)
	}
	if c := status["radioBouquets"]; c.Status != CheckOK || c.Params["first"] != "Favourites (Radio)" {
		t.Errorf("radio root: %+v", c)
	}
	// A web radio stream (4097:…, type 2) counts as a station; a TV channel
	// in a radio bouquet doesn't and is flagged.
	if c := status["radioStations"]; c.Status != CheckWarn || c.Code != "radioOther" || c.Params["stations"] != 2 || c.Params["other"] != 1 {
		t.Errorf("radio stations: %+v", c)
	}
	if c := status["writes"]; c.Status != CheckWarn {
		t.Errorf("writes off by default here: %+v", c)
	}
	for _, name := range []string{"deviceinfo.xml", "getservices.xml", "getcurrent.xml", "getservices_radio_root.xml", "getservices_radio_bouquet.xml", "epgnownext_radio.xml", "timerlist.xml", "powerstate.xml"} {
		if !slices.Contains(rep.Captured, name) {
			t.Errorf("%s not captured: %v", name, rep.Captured)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not on disk: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "report.json")); err != nil {
		t.Error("report.json not written")
	}
	if st.Last() != rep {
		t.Error("Last should return this run")
	}
}

func TestSelfTest_BoxDownStillChecksTheDisk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer srv.Close()
	st := &SelfTest{WC: webcontrol.NewHTTPClient(srv.URL), Dir: filepath.Join(t.TempDir(), "selftest"), PiconDirs: []string{"/nonexistent"}}
	rep, err := st.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range rep.Checks {
		ids = append(ids, c.ID)
	}
	if rep.Checks[0].Status != CheckFail || slices.Contains(ids, "radioBouquets") || !slices.Contains(ids, "picons") || !slices.Contains(ids, "services") {
		t.Errorf("box down: %v %+v", ids, rep.Checks[0])
	}
}

// The reference box's list of all radio services (fixture, captured by the
// self-test 2026-10-04): 664 entries; after dropping data services ("."),
// names Enigma2 made up ("10796V SID 0x3dae") and stations on a second
// transponder, each station once, 1LIVE among them.
func TestMergeStations_ReferenceBoxRadioList(t *testing.T) {
	b, err := os.ReadFile("../../testdata/fixtures/getservices_radio_all.xml")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer srv.Close()
	services, err := webcontrol.NewHTTPClient(srv.URL).Services(context.Background(), webcontrol.RadioAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 664 {
		t.Fatalf("fixture has %d entries", len(services))
	}
	stations := mergeStations(services, nil, true)
	seen := map[string]bool{}
	var live bool
	for _, st := range stations {
		if unnamedService(st.ServiceName) || strings.Contains(st.ServiceName, " SID 0x") {
			t.Errorf("unnamed service kept: %q", st.ServiceName)
		}
		if seen[strings.ToLower(st.ServiceName)] {
			t.Errorf("duplicate %q", st.ServiceName)
		}
		seen[strings.ToLower(st.ServiceName)] = true
		live = live || st.ServiceName == "1LIVE"
	}
	if !live || len(stations) < 400 || len(stations) >= 664 {
		t.Errorf("%d stations, 1LIVE among them: %v", len(stations), live)
	}
}
