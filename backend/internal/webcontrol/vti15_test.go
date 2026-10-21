package webcontrol

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Answers recorded from a VU+ Solo2 running VTi 15.0.04 (OWIF 1.5.2) on
// 2026-10-21, see ARCHITECTURE.md ("VU+ Solo2 with VTi 15"). The client must
// read an OpenWebif box exactly like a Dreambox.
func vti15Box(t *testing.T) *HTTPClient {
	t.Helper()
	files := map[string]string{
		"/web/deviceinfo":  "vti15_deviceinfo.xml",
		"/web/getcurrent":  "vti15_getcurrent.xml",
		"/web/getservices": "vti15_getservices.xml",
		"/web/about":       "vti15_about.xml",
		"/web/powerstate":  "vti15_powerstate.xml",
		"/web/vol":         "vti15_vol.xml",
		"/web/movielist":   "vti15_movielist.xml",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile("../../testdata/fixtures/" + name)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/xml")
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL)
}

func TestVTi15_DeviceInfo(t *testing.T) {
	d, err := vti15Box(t).DeviceInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.DeviceName != "Solo²" || d.WebifVersion != "OWIF 1.5.2" || d.ImageVersion != "15.0.04" || d.Tuners() != 2 || len(d.HDDs) != 1 {
		t.Errorf("device info: %+v", d)
	}
}

func TestVTi15_CurrentTakesTheVolumeFromVol(t *testing.T) {
	cur, err := vti15Box(t).Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cur.ServiceName != "Das Erste HD" || cur.Volume != 30 || cur.Muted || cur.Now != nil {
		t.Errorf("current: %+v", cur)
	}
}

func TestVTi15_BouquetsStandbyAndMovies(t *testing.T) {
	c := vti15Box(t)
	bq, err := c.Bouquets(context.Background())
	if err != nil || len(bq) != 3 || bq[0].ServiceName != "Free TV" {
		t.Errorf("bouquets: %v %v", bq, err)
	}
	if standby, err := c.InStandby(context.Background()); err != nil || standby {
		t.Errorf("standby: %v %v", standby, err)
	}
	movies, err := c.MovieList(context.Background())
	if err != nil || len(movies) != 2 || movies[0].Title == "" || movies[0].FileSize == 0 {
		t.Errorf("movies: %+v %v", movies, err)
	}
}

// On an OpenWebif box "every channel" comes from its bouquets: the query for
// every service ordered by name took the VU+ Solo2 off the network.
func TestOpenWebif_EveryChannelFromTheBouquets(t *testing.T) {
	const entry = `<e2service><e2servicereference>%s</e2servicereference><e2servicename>%s</e2servicename></e2service>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/web/deviceinfo" {
			b, _ := os.ReadFile("../../testdata/fixtures/vti15_deviceinfo.xml")
			w.Write(b)
			return
		}
		if r.URL.Path != "/web/getservices" {
			http.NotFound(w, r)
			return
		}
		switch ref := r.URL.Query().Get("sRef"); {
		case ref == "":
			fmt.Fprintf(w, "<e2servicelist>"+entry+entry+"</e2servicelist>", `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "a.tv" ORDER BY bouquet`, "A", `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "b.tv" ORDER BY bouquet`, "B")
		case strings.Contains(ref, `"a.tv"`):
			fmt.Fprintf(w, "<e2servicelist>"+entry+entry+"</e2servicelist>", "1:0:19:2:1:1:C00000:0:0:0:", "Zeta HD", "1:0:19:1:1:1:C00000:0:0:0:", "alpha HD")
		case strings.Contains(ref, `"b.tv"`):
			fmt.Fprintf(w, "<e2servicelist>"+entry+entry+"</e2servicelist>", "1:0:19:1:1:1:C00000:0:0:0:", "alpha HD", "1:0:19:3:1:1:C00000:0:0:0:", "Mid HD")
		default:
			t.Errorf("a query for more than the bouquets reached the box: %s", ref)
		}
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	all, err := c.Services(context.Background(), TVAll)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range all {
		names = append(names, s.ServiceName)
	}
	if got := strings.Join(names, ","); got != "alpha HD,Mid HD,Zeta HD" {
		t.Errorf("channels = %s, want alpha HD,Mid HD,Zeta HD (once each, by name)", got)
	}
	if p, err := c.Services(context.Background(), TVProviders); err != nil || len(p) != 0 {
		t.Errorf("providers = %v, %v; want none", p, err)
	}
}

func TestVTi15_TunerStates(t *testing.T) {
	states, err := vti15Box(t).TunerStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0].Name != "Tuner A" || states[0].Live != "19.2° O / Das Erste HD" || !states[0].Busy() || states[1].Busy() {
		t.Errorf("tuner states: %+v", states)
	}
}

// A Dreambox's /web/about has no tuner section: no states, no error.
func TestTunerStates_NoneOnADreambox(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<e2abouts><e2about><e2model>one</e2model></e2about></e2abouts>`))
	}))
	defer srv.Close()
	states, err := NewHTTPClient(srv.URL).TunerStates(context.Background())
	if err != nil || len(states) != 0 {
		t.Errorf("states %v, err %v", states, err)
	}
}

// No DreamHub plugin on an OpenWebif box: a recording is played by zapping
// to its reference.
func TestVTi15_PlayMovieByZap(t *testing.T) {
	var zapped string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/web/deviceinfo":
			b, _ := os.ReadFile("../../testdata/fixtures/vti15_deviceinfo.xml")
			w.Write(b)
		case "/web/zap":
			zapped = r.URL.Query().Get("sRef")
			w.Write([]byte(`<e2simplexmlresult><e2state>True</e2state><e2statetext>Active service is now</e2statetext></e2simplexmlresult>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	ref := "1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/a.ts"
	if err := c.PlayMovie(context.Background(), ref, "A", 0); err != nil {
		t.Fatal(err)
	}
	if zapped != ref {
		t.Errorf("zapped to %q, want %q", zapped, ref)
	}
}
