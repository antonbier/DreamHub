package logging

import (
	"testing"
	"time"
)

func TestDebugRevertsAutomatically(t *testing.T) {
	l := NewLevel(nil)
	l.SetDebug(30 * time.Millisecond)
	if level, until := l.Get(); level != "debug" || until.IsZero() {
		t.Fatalf("expected debug with a revert time, got %s %v", level, until)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if level, _ := l.Get(); level == "info" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("debug must revert to info on its own")
}
