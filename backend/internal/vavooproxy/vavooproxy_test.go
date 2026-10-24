package vavooproxy

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsure_AlreadyListening(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	m := &Manager{Addr: l.Addr().String(), Script: "/nonexistent"}
	if err := m.Ensure(context.Background()); err != nil {
		t.Fatalf("a running proxy needs nothing: %v", err)
	}
}

func TestEnsure_NoScript(t *testing.T) {
	m := &Manager{Addr: "127.0.0.1:1", Script: filepath.Join(t.TempDir(), "missing.py")}
	if err := m.Ensure(context.Background()); err != ErrNoScript {
		t.Fatalf("got %v", err)
	}
}

// A script that opens the port is started once and then used.
func TestEnsure_StartsTheScript(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close() // free port: nothing listens yet
	script := filepath.Join(t.TempDir(), "proxy.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	started := 0
	var srv net.Listener
	m := &Manager{Addr: addr, Script: script, Wait: 3 * time.Second, Command: func(s string) *exec.Cmd {
		started++
		// stand-in for the script: open the port a moment after the process starts
		go func() {
			time.Sleep(300 * time.Millisecond)
			srv, _ = net.Listen("tcp", addr)
		}()
		return exec.Command("sh", s)
	}}
	defer m.Stop()
	if err := m.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if srv != nil {
		defer srv.Close()
	}
	if err := m.Ensure(context.Background()); err != nil || started != 1 {
		t.Fatalf("second call: %v, started %d times", err, started)
	}
}

func TestEnsure_ScriptThatNeverListensFails(t *testing.T) {
	script := filepath.Join(t.TempDir(), "p.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	m := &Manager{Addr: "127.0.0.1:1", Script: script, Wait: 600 * time.Millisecond}
	if err := m.Ensure(context.Background()); err == nil {
		t.Fatal("no port, no success")
	}
	// and it is not tried again at once
	start := time.Now()
	if err := m.Ensure(context.Background()); err == nil || time.Since(start) > 400*time.Millisecond {
		t.Fatalf("retried too soon: %v after %s", err, time.Since(start))
	}
}
