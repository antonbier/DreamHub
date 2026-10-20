package radiotext

import (
	"bytes"
	"os"
	"testing"
)

// A real stretch of Dlf Kultur from the DVB-C reference box (2026-10-20),
// the audio itself replaced by silence: only the RDS bytes are the
// station's.
func TestRead_RealStation(t *testing.T) {
	f, err := os.Open("testdata/dlf-kultur.ts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := Read(f, 1<<20)
	want := "Proms London: Mahlers 9. Sinfonie im Originalklang, Philipp von"
	if err != nil || got != want {
		t.Fatalf("Read = %q, %v; want %q", got, err, want)
	}
}

func TestRead_NoText(t *testing.T) {
	// A transport stream of nothing but padding packets.
	pad := append([]byte{0x47, 0x1F, 0xFF, 0x10}, make([]byte, 184)...)
	got, err := Read(bytes.NewReader(bytes.Repeat(pad, 50)), 1<<20)
	if got != "" || err != nil {
		t.Fatalf("Read = %q, %v", got, err)
	}
}

func TestLastText_UnstuffsAndTakesTheLast(t *testing.T) {
	msg := func(text string) []byte {
		body := append([]byte{0x00, 0x00, 0x01, byte(len(text) + 4), 0x0A, 0x00, 0x00, byte(len(text) + 1), 0x01}, text...)
		body = append(body, 0x12, 0x34) // CRC, not checked
		return append(append([]byte{0xFE}, body...), 0xFF)
	}
	data := append(append([]byte{0x55, 0x55}, msg("First")...), msg("Second one")...)
	if got := lastText(data); got != "Second one" {
		t.Fatalf("got %q", got)
	}
	if got := lastText(msg("Half")[:6]); got != "" {
		t.Fatalf("an unfinished message gave %q", got)
	}
}
