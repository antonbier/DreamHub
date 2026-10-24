package hubs

import (
	"context"
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
