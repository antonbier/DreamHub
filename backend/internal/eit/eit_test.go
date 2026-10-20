package eit

import (
	"encoding/hex"
	"testing"
	"time"
)

// The start of "20230101 2019 - VOX - Yesterday.eit" from the reference
// box: a film announced for 19:15 UTC on 1 January 2023, 2 h 20 min long.
func TestParse_RealFile(t *testing.T) {
	b, _ := hex.DecodeString("ce93ea29191500022000" + "00e94d106765720a1559657374657264")
	ev, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Start.Equal(time.Date(2023, 1, 1, 19, 15, 0, 0, time.UTC)) || ev.Duration != 140*time.Minute || ev.ID != 0xce93 {
		t.Fatalf("event: %+v", ev)
	}
}

func TestParse_Refuses(t *testing.T) {
	if _, err := Parse([]byte{1, 2, 3}); err != ErrShort {
		t.Fatalf("short: %v", err)
	}
	// 0xFF is no BCD digit: undefined start/duration (a "running later" event).
	if _, err := Parse([]byte{0, 1, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02, 0x20, 0x00}); err != ErrInvalid {
		t.Fatalf("invalid: %v", err)
	}
}
