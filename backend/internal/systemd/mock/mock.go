// Package mock implements systemd.Manager in memory: the services found
// running on the reference box (ARCHITECTURE.md, "Process architecture"),
// one of them failed. Restart brings
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
// except nmb, failed here so the screen has something to show.
func New() *Manager {
	up := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	// The reference box's units as `systemctl show` reported them live
	// (2026-10-04, systemd 232): no memory accounting, no NRestarts.
	units := []systemd.Unit{
		{Name: "enigma2.service", Description: "enigma2", Active: "active", Sub: "running", Since: up, MainPID: 812},
		{Name: "dreamhub.service", Description: "DreamHub", Active: "active", Sub: "running", Since: up.Add(40 * time.Second), MainPID: 1204},
		{Name: "smb.service", Description: "Samba SMB Daemon", Active: "active", Sub: "running", Since: up, MainPID: 640},
		{Name: "nmb.service", Description: "Samba NMB Daemon", Active: "failed", Sub: "failed"},
		{Name: "nfs-server.service", Description: "NFS server and services", Active: "active", Sub: "exited", Since: up},
		{Name: "avahi-daemon.service", Description: "Avahi mDNS/DNS-SD Stack", Active: "active", Sub: "running", Since: up, MainPID: 402},
		{Name: "zerotierone.service", Description: "ZeroTier One", Active: "active", Sub: "running", Since: up, MainPID: 455},
		{Name: "oscam-cycecplus.service", Description: "OSCam", Active: "active", Sub: "running", Since: up, MainPID: 470},
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
