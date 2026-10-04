package hubs

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func TestPiconNames(t *testing.T) {
	got := piconNames("1:0:19:2b7a:3F3:1:C00000:0:0:0:", "Das Erste HD")
	want := []string{"1_0_19_2B7A_3F3_1_C00000_0_0_0.png", "1_0_1_2B7A_3F3_1_C00000_0_0_0.png", "daserstehd.png"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("got %q", got)
	}
	if got := piconNames("../../etc/passwd", "../"); len(got) != 0 {
		t.Errorf("nothing from a bogus reference or name: %q", got)
	}
}

func TestGuideViews(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	os.WriteFile(filepath.Join(dir, "1_0_1_2B7A_3F3_1_C00000_0_0_0.png"), b.Bytes(), 0o644)
	os.WriteFile(filepath.Join(dir, "rtltelevision.png"), []byte("not a picture"), 0o644)

	h := &Guide{WC: mock.New(), PiconDirs: []string{dir}}
	mux := http.NewServeMux()
	h.Mount(mux)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}

	if rec := get("/api/v1/guide/picon?ref=" + url.QueryEscape("1:0:19:2B7A:3F3:1:C00000:0:0:0:") + "&name=zdf_neo"); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("the type-1 picon is found: %d", rec.Code)
	}
	if rec := get("/api/v1/guide/picon?ref=x&name=RTL+Television"); rec.Code != 404 {
		t.Errorf("only real PNGs are served: %d", rec.Code)
	}

	bRef := url.QueryEscape(`1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`)
	rec := get("/api/v1/guide/nownext?bouquet=" + bRef)
	var nn struct {
		Channels []struct {
			ServiceName string `json:"serviceName"`
			Now         *struct {
				Title string `json:"title"`
			} `json:"now"`
		} `json:"channels"`
	}
	json.Unmarshal(rec.Body.Bytes(), &nn)
	if rec.Code != 200 || len(nn.Channels) != 3 || nn.Channels[1].Now == nil || nn.Channels[1].Now.Title != "The Rookie" {
		t.Errorf("now/next per channel: %d %s", rec.Code, rec.Body)
	}

	start, end := mock.Evening.Unix(), mock.Evening.Unix()+86400
	rec = get("/api/v1/guide/grid?bouquet=" + bRef + "&start=" + strconv.FormatInt(start, 10) + "&end=" + strconv.FormatInt(end, 10) + "&service=" + url.QueryEscape("1:0:19:283D:3FB:1:C00000:0:0:0:"))
	var grid struct {
		Channels []struct {
			ServiceName string `json:"serviceName"`
		} `json:"channels"`
	}
	json.Unmarshal(rec.Body.Bytes(), &grid)
	if len(grid.Channels) != 1 || grid.Channels[0].ServiceName != "Das Erste HD" {
		t.Errorf("one channel only: %s", rec.Body)
	}
}

// A picon pool with sets in subfolders (the reference box's layout): the
// logo is found in a set, the newest set first.
func TestPiconSearchDirs_LooksIntoSets(t *testing.T) {
	pool := t.TempDir()
	older := filepath.Join(pool, "PPH_carbon_220x132")
	newer := filepath.Join(pool, "stefanbenno6_220x132")
	for _, d := range []string{older, newer} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(older, past, past)
	got := piconSearchDirs([]string{pool, "/nonexistent"})
	want := []string{pool, newer, older, "/nonexistent"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
