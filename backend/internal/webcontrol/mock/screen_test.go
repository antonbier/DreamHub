package mock

import (
	"bytes"
	"context"
	"image/png"
	"testing"
)

func TestScreen_KeysChangeWhatTheScreenshotShows(t *testing.T) {
	c := New()
	ctx := context.Background()
	shot := func() []byte {
		t.Helper()
		b, err := c.Screenshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := png.Decode(bytes.NewReader(b)); err != nil {
			t.Fatalf("not a PNG: %v", err)
		}
		return b
	}

	plain := shot()
	c.SendKey(ctx, keyMenu)
	menu := shot()
	if bytes.Equal(plain, menu) {
		t.Error("MENU should open the simulated menu overlay")
	}
	c.SendKey(ctx, keyDown)
	if bytes.Equal(menu, shot()) {
		t.Error("DOWN should move the menu highlight")
	}
	c.SendKey(ctx, keyExit)
	if !bytes.Equal(plain, shot()) {
		t.Error("EXIT should close the menu again")
	}

	c.SendKey(ctx, keyPower)
	if standby, _ := c.InStandby(ctx); !standby {
		t.Error("POWER should put the mock box into standby")
	}
	c.SendKey(ctx, keyMenu) // ignored in standby, like the real box
	c.SendKey(ctx, keyPower)
	if !bytes.Equal(plain, shot()) {
		t.Error("keys other than POWER must be ignored in standby")
	}
}
