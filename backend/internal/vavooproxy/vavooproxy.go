// Package vavooproxy starts the proxy of the Netcast Vavoo addon when it is
// not running (docs/NETCAST_PLAN.md, "Vavoo").
//
// The addon plays every Vavoo channel through a small web server on
// 127.0.0.1:4323 and starts it only when its Vavoo screen is opened at the TV
// (vavooapi_lister.py calls start_proxy_background()); after a restart of
// enigma2 nothing listens, and the box's player then says "unknown media type"
// for every Vavoo channel (found on the One, 2026-10-24: all plays worked until
// enigma2 restarted, none after). The proxy module is also a script of its own
// — run directly it serves the same port — so DreamHub, which runs on the box,
// starts that script, the addon's own code unchanged, exactly when a Vavoo
// channel is about to be played. It lives as long as DreamHub's service.
package vavooproxy

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	// Script is the addon's proxy on the Dreambox One.
	Script = "/usr/lib/enigma2/python/Plugins/GP4/lib/vavooapi_proxy.py"
	// Addr is where it listens.
	Addr = "127.0.0.1:4323"
)

// ErrNoScript: the addon is not installed on this machine.
var ErrNoScript = errors.New("vavooproxy: the Netcast Vavoo addon is not installed here")

// Manager starts the proxy on demand. The zero value uses the real paths.
type Manager struct {
	Script string // "" means Script
	Addr   string // "" means Addr
	// Wait is how long a start is given to open the port; 0 means 10 s.
	Wait time.Duration
	// Command builds the process to run; nil runs python on the script.
	Command func(script string) *exec.Cmd

	mu       sync.Mutex
	cmd      *exec.Cmd
	failedAt time.Time
}

func (m *Manager) script() string {
	if m.Script != "" {
		return m.Script
	}
	return Script
}

func (m *Manager) addr() string {
	if m.Addr != "" {
		return m.Addr
	}
	return Addr
}

func (m *Manager) listening() bool {
	c, err := net.DialTimeout("tcp", m.addr(), 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Ensure returns once the proxy answers on its port, starting the addon's
// script if nothing does. ErrNoScript when the addon is not there.
func (m *Manager) Ensure(ctx context.Context) error {
	if m.listening() {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listening() { // another request started it meanwhile
		return nil
	}
	if _, err := os.Stat(m.script()); err != nil {
		return ErrNoScript
	}
	if !m.failedAt.IsZero() && time.Since(m.failedAt) < 30*time.Second {
		return errors.New("vavooproxy: the proxy did not start a moment ago")
	}
	cmd := m.command()
	if err := cmd.Start(); err != nil {
		m.failedAt = time.Now()
		return err
	}
	m.cmd = cmd
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
		}
		m.mu.Unlock()
	}()
	wait := m.Wait
	if wait <= 0 {
		wait = 10 * time.Second
	}
	deadline := time.After(wait)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if m.listening() {
				return nil
			}
		case <-exited:
			// the port may be taken by the addon's own copy inside enigma2
			if m.listening() {
				return nil
			}
			m.failedAt = time.Now()
			return errors.New("vavooproxy: the proxy stopped right after starting")
		case <-deadline:
			cmd.Process.Kill()
			m.failedAt = time.Now()
			return errors.New("vavooproxy: the proxy did not open its port")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (m *Manager) command() *exec.Cmd {
	if m.Command != nil {
		return m.Command(m.script())
	}
	python := "python"
	if p, err := exec.LookPath("python"); err == nil {
		python = p
	} else if p, err := exec.LookPath("python3"); err == nil {
		python = p
	}
	cmd := exec.Command(python, m.script())
	cmd.Dir = os.TempDir()
	return cmd
}

// Stop ends a proxy this manager started (at shutdown).
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
}
