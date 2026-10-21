package webcontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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
