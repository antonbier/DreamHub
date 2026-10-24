package hubs

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"bytes"
	"context"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// A box without the Netcast plugin (a Solo2) has none of its files: the page
// is then left out of the navigation. A box that cannot be asked is not a
// reason to hide it.
func TestNetcastPresent(t *testing.T) {
	m := mock.New()
	h := &Netcast{Channels: &Channels{WC: m}}
	if !h.Present(context.Background()) {
		t.Error("a box with Netcast's files has it")
	}
	for _, p := range []string{"/etc/enigma2/netcastserver_gemini.json", "/etc/enigma2/netcastserver_plutotv.json", "/etc/enigma2/netcastepg_gemini.json"} {
		delete(m.Files, p)
	}
	h2 := &Netcast{Channels: &Channels{WC: m}}
	if h2.Present(context.Background()) {
		t.Error("a box without any source file has no Netcast")
	}
}

// Vavoo's logo host refuses the box, so a channel without a reachable logo
// gets the picon the box has under its name.
func TestNetcastPiconByName(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	if err := os.WriteFile(filepath.Join(dir, "skycinemapremierenhd.png"), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Netcast{Channels: &Channels{PiconDirs: []string{dir}}}
	if got := h.piconByName("SKY CINEMA PREMIEREN [SAT] []"); !bytes.Equal(got, b.Bytes()) {
		t.Errorf("name with notes: %d bytes", len(got))
	}
	if h.piconByName("Unknown Channel") != nil || h.piconByName("[SAT]") != nil {
		t.Error("no picon, no answer")
	}
}

func TestNetcastPlay_StartsTheVavooProxyOnlyForVavoo(t *testing.T) {
	m := mock.New()
	calls := 0
	h := &Netcast{Channels: &Channels{WC: m, Now: func() time.Time { return time.Unix(1, 0) }}, VavooProxy: func(context.Context) error { calls++; return nil }}
	user := store.User{ID: 1}
	sess := &iptvSession{token: "t", userID: user.ID, read: time.Unix(1, 0), pl: &iptv.Playlist{Entries: []iptv.Entry{
		{Name: "Vavoo ch", URL: netcastVavooProxy + "https%3A%2F%2Fvavoo.to%2Fplay%2Fabc"},
		{Name: "Plain", URL: "https://tv.example/a.m3u8"},
	}}}
	h.Channels.keepIPTV(sess)
	play := func(id int) map[string]any {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/v1/netcast/play", strings.NewReader(`{"token":"t","id":`+strconv.Itoa(id)+`}`))
		req = req.WithContext(httpx.WithUser(req.Context(), user))
		h.play(rec, req)
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	if out := play(1); calls != 0 || out["warning"] != nil {
		t.Errorf("a plain stream needs no proxy: %d %v", calls, out)
	}
	if out := play(0); calls != 1 || out["warning"] != nil {
		t.Errorf("a Vavoo channel makes sure of it: %d %v", calls, out)
	}
	h.VavooProxy = func(context.Context) error { return errors.New("no") }
	if out := play(0); out["warning"] != "vavoo_proxy" {
		t.Errorf("a failed start warns: %v", out)
	}
}
