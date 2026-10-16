package boxes

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

func TestParseMAC(t *testing.T) {
	for in, want := range map[string]string{
		"AA:BB:CC:DD:EE:FF": "aa:bb:cc:dd:ee:ff",
		"aa-bb-cc-dd-ee-ff": "aa:bb:cc:dd:ee:ff",
		"aabbccddeeff":      "aa:bb:cc:dd:ee:ff",
		" aabb.ccdd.eeff ":  "aa:bb:cc:dd:ee:ff",
		"aa:bb:cc:dd:ee":    "",
		"not a mac":         "",
		"":                  "",
		"aa:bb:cc:dd:ee:gg": "",
	} {
		if got := ParseMAC(in); got != want {
			t.Errorf("ParseMAC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMagicPacket(t *testing.T) {
	p, err := MagicPacket("aa:bb:cc:dd:ee:ff")
	if err != nil || len(p) != 102 {
		t.Fatalf("len %d err %v", len(p), err)
	}
	if !bytes.Equal(p[:6], bytes.Repeat([]byte{0xFF}, 6)) {
		t.Error("starts with six 0xFF")
	}
	for i := 0; i < 16; i++ {
		if !bytes.Equal(p[6+i*6:12+i*6], []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}) {
			t.Fatalf("repetition %d wrong", i)
		}
	}
	if _, err := MagicPacket("nope"); err == nil {
		t.Error("an invalid MAC must fail")
	}
}

func s(v string) *string { return &v }

func TestApplyWake(t *testing.T) {
	var b store.Box
	if err := apply(&b, Input{WakeMethod: s("wol"), WakeMAC: s("AA-BB-CC-DD-EE-FF")}, false); err != nil {
		t.Fatal(err)
	}
	if b.WakeMethod != "wol" || b.WakeMAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("wol not stored: %+v", b)
	}
	// Wake-on-LAN without a valid MAC is refused and changes nothing.
	b2 := b
	err := apply(&b2, Input{WakeMAC: s("zz")}, false)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Fields["wakeMac"] != "invalid" {
		t.Fatalf("want wakeMac invalid, got %v", err)
	}
	// An extension needs both its id and the action.
	err = apply(&b2, Input{WakeMethod: s("extension"), WakeExt: s("ewelink")}, false)
	if !errors.As(err, &ve) || ve.Fields["wakeAction"] != "required" {
		t.Fatalf("want wakeAction required, got %v", err)
	}
	if err := apply(&b, Input{WakeMethod: s("extension"), WakeExt: s("ewelink"), WakeAction: s("toggle-1")}, false); err != nil {
		t.Fatal(err)
	}
	if b.WakeMAC != "" || b.WakeExt != "ewelink" {
		t.Fatalf("switching method must forget the MAC: %+v", b)
	}
	// "none" forgets everything; an unknown method is refused. Also for the first box.
	if err := apply(&b, Input{WakeMethod: s("")}, true); err != nil {
		t.Fatal(err)
	}
	if b.WakeMethod != "" || b.WakeExt != "" || b.WakeAction != "" {
		t.Fatalf("none must clear: %+v", b)
	}
	if err := apply(&b, Input{WakeMethod: s("magic")}, false); err == nil {
		t.Error("an unknown method must be refused")
	}
}

func TestManagerWake(t *testing.T) {
	r := NewRegistry()
	r.Add(store.Box{ID: "main", Name: "main"}, Clients{}, true)
	r.Add(store.Box{ID: "wz", Name: "wz", WakeMethod: WakeWOL, WakeMAC: "aa:bb:cc:dd:ee:ff"}, Clients{}, false)
	var sent, woken []string
	m := &Manager{Registry: r, SendWOL: func(mac string) error { sent = append(sent, mac); return nil }, OnWake: func(id string) { woken = append(woken, id) }}
	if err := m.Wake(context.Background(), "main"); !errors.Is(err, ErrNoWake) {
		t.Fatalf("no method set: %v", err)
	}
	if err := m.Wake(context.Background(), "nope"); !errors.Is(err, ErrUnknownBox) {
		t.Fatalf("unknown box: %v", err)
	}
	if err := m.Wake(context.Background(), "wz"); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != "aa:bb:cc:dd:ee:ff" || len(woken) != 1 || woken[0] != "wz" {
		t.Fatalf("sent %v woken %v", sent, woken)
	}
	m.SendWOL = func(string) error { return ErrWakeSend }
	if err := m.Wake(context.Background(), "wz"); !errors.Is(err, ErrWakeSend) || len(woken) != 1 {
		t.Fatalf("a failed send must not boost: %v %v", err, woken)
	}
}
