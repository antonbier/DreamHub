package hubs

import (
	"bytes"
	"context"
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
