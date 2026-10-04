// Package mock implements systemd.Manager in memory: the services found
// running on the reference box (ARCHITECTURE.md, "Process architecture"),
// one of them failed so the screen has something to show. Restart brings
// a unit (back) up and records it for tests.
package mock

import (
	"context"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/systemd"
)

// Manager is an in-memory systemd.Manager.
type Manager struct {
	mu    sync.Mutex
	units map[string]systemd.Unit
	// Now is the mock's clock; nil means time.Now.
	Now func() time.Time
	// Restarted lists the units restarted, in order.
	Restarted []string
}

var _ systemd.Manager = (*Manager)(nil)

// New returns the reference box's services, up since about two hours,
// except nmbd, which failed.
func New() *Manager {
	up := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	units := []systemd.Unit{
		{Name: "enigma2.service", Description: "enigma2", Active: "active", Sub: "running", Since: up, MainPID: 812, Memory: 182 << 20},
		{Name: "dreamhub.service", Description: "DreamHub", Active: "active", Sub: "running", Since: up.Add(40 * time.Second), MainPID: 1204, Memory: 24 << 20},
		{Name: "smbd.service", Description: "Samba SMB Daemon", Active: "active", Sub: "running", Since: up, MainPID: 640, Memory: 9 << 20},
		{Name: "nmbd.service", Description: "Samba NMB Daemon", Active: "failed", Sub: "failed", Restarts: 3},
		{Name: "avahi-daemon.service", Description: "Avahi mDNS/DNS-SD Stack", Active: "active", Sub: "running", Since: up, MainPID: 402, Memory: 2 << 20},
		{Name: "zerotier-one.service", Description: "ZeroTier One", Active: "active", Sub: "running", Since: up, MainPID: 455, Memory: 6 << 20},
	}
	m := &Manager{units: map[string]systemd.Unit{}}
	for _, u := range units {
		m.units[u.Name] = u
	}
	return m
}

func (m *Manager) Status(ctx context.Context, names []string) ([]systemd.Unit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []systemd.Unit{}
	for _, n := range names {
		if u, ok := m.units[n]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

func (m *Manager) Restart(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now
	if m.Now != nil {
		now = m.Now
	}
	u := m.units[name]
	u.Name, u.Active, u.Sub, u.Since = name, "active", "running", now().Truncate(time.Second)
	if u.MainPID == 0 {
		u.MainPID = 2000 + len(m.Restarted)
	} else {
		u.MainPID++
	}
	m.units[name] = u
	m.Restarted = append(m.Restarted, name)
	return nil
}
