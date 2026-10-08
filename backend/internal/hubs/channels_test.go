package hubs

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// A tiny valid PNG (1×1), so readPicon's content check passes.
var onePixelPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")

func TestChannels_PiconSetsCoverageAndChoice(t *testing.T) {
	pool := t.TempDir()
	rtl := piconNames(mock.TVRTL.ServiceReference, mock.TVRTL.ServiceName)[0]
	zdfneo := piconNames(mock.TVZDFNeo.ServiceReference, mock.TVZDFNeo.ServiceName)[0]
	write := func(set, file string, b []byte) {
		os.MkdirAll(filepath.Join(pool, set), 0o755)
		os.WriteFile(filepath.Join(pool, set, file), b, 0o644)
	}
	write("full", rtl, onePixelPNG)
	write("full", zdfneo, onePixelPNG)
	write("partial", rtl, append(append([]byte{}, onePixelPNG...), 'x')) // same channel, other file

	h := &Channels{WC: mock.New(), PiconDirs: []string{pool}}
	rec := httptest.NewRecorder()
	h.picons(rec, httptest.NewRequest("GET", "/api/v1/channels/picons", nil))
	var body struct {
		Sets []struct {
			Name    string `json:"name"`
			Covered int    `json:"covered"`
		} `json:"sets"`
		Total int `json:"total"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Sets) != 2 || body.Sets[0].Name != "full" || body.Sets[0].Covered != 2 || body.Sets[1].Covered != 1 {
		t.Fatalf("sets: %+v", body)
	}

	// The chosen set is searched first.
	SetPreferredPiconSet("partial")
	t.Cleanup(func() { SetPreferredPiconSet("") })
	b, ok := readPicon([]string{pool}, mock.TVRTL.ServiceReference, mock.TVRTL.ServiceName)
	if !ok || b[len(b)-1] != 'x' {
		t.Fatal("RTL's logo should come from the chosen set")
	}
	SetPreferredPiconSet("full")
	if b, _ := readPicon([]string{pool}, mock.TVRTL.ServiceReference, mock.TVRTL.ServiceName); b[len(b)-1] == 'x' {
		t.Fatal("after choosing another set, its logo comes first")
	}
}
