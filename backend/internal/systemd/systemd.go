// Package systemd reads and restarts system services for AdminHub's
// service status (ROADMAP.md, v1.1 "AdminHub service status"). The box
// runs full systemd (ARCHITECTURE.md, "Runtime: systemd") and DreamHub runs
// there as root, so systemctl answers directly. Like internal/journald:
// Exec shells out for real, ./mock answers from memory.
package systemd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Unit is one service's state, trimmed to what AdminHub shows.
type Unit struct {
	Name        string `json:"name"` // "enigma2.service"
	Description string `json:"description"`
	// Active is systemd's ActiveState: active, inactive, failed,
	// activating, deactivating, reloading.
	Active string `json:"active"`
	Sub    string `json:"sub"` // running, exited, dead, …
	// Since is when the unit last entered the active state; zero when it
	// isn't active.
	Since    time.Time `json:"since,omitzero"`
	MainPID  int       `json:"mainPid,omitempty"`
	Memory   int64     `json:"memory,omitempty"` // bytes; 0 when unknown
	Restarts int       `json:"restarts"`         // automatic restarts by systemd (NRestarts)
}

// Manager reads unit states and restarts units.
type Manager interface {
	// Status returns the units of names that exist on this system, in the
	// order asked; unknown names are left out.
	Status(ctx context.Context, names []string) ([]Unit, error)
	// Restart (re)starts a unit without waiting for it — so DreamHub can
	// restart itself and still answer.
	Restart(ctx context.Context, name string) error
}

// ErrUnavailable means systemctl isn't on this system (a development
// machine without systemd).
var ErrUnavailable = errors.New("systemd: systemctl not available")

// Exec runs systemctl.
type Exec struct{}

var showProps = []string{"Id", "Description", "LoadState", "ActiveState", "SubState", "ActiveEnterTimestampMonotonic", "MainPID", "MemoryCurrent", "NRestarts"}

func (Exec) Status(ctx context.Context, names []string) ([]Unit, error) {
	if len(names) == 0 {
		return []Unit{}, nil
	}
	out, err := Exec{}.Show(ctx, names)
	if err != nil {
		return nil, err
	}
	return ParseShow(out, bootTime()), nil
}

// Show is `systemctl show` for names as systemctl prints it (the
// self-test keeps it as a fixture).
func (Exec) Show(ctx context.Context, names []string) ([]byte, error) {
	args := append([]string{"show", "--property=" + strings.Join(showProps, ",")}, names...)
	out, err := exec.CommandContext(ctx, "systemctl", args...).Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("systemd: systemctl show: %w", err)
	}
	return out, nil
}

func (Exec) Restart(ctx context.Context, name string) error {
	out, err := exec.CommandContext(ctx, "systemctl", "restart", "--no-block", name).CombinedOutput()
	if errors.Is(err, exec.ErrNotFound) {
		return ErrUnavailable
	}
	if err != nil {
		return fmt.Errorf("systemd: restart %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// bootTime is when the system booted, from /proc/uptime: systemd's
// monotonic timestamps count from there. The wall-clock variant
// (ActiveEnterTimestamp) comes as a local, zone-abbreviated string that
// older systemd versions can't be asked to print otherwise.
func bootTime() time.Time {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return time.Time{}
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return time.Time{}
	}
	up, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return time.Time{}
	}
	return time.Now().Add(-time.Duration(up * float64(time.Second)))
}

// ParseShow reads `systemctl show --property=…` output for several units:
// one block of Key=Value lines per unit, blocks separated by a blank line.
// Units systemd doesn't know (LoadState=not-found) are left out. boot is
// the boot time the monotonic timestamps are added to; zero leaves Since
// unset.
func ParseShow(out []byte, boot time.Time) []Unit {
	units := []Unit{}
	var cur map[string]string
	flush := func() {
		if cur == nil {
			return
		}
		if cur["LoadState"] != "not-found" && cur["Id"] != "" {
			u := Unit{Name: cur["Id"], Description: cur["Description"], Active: cur["ActiveState"], Sub: cur["SubState"]}
			u.MainPID, _ = strconv.Atoi(cur["MainPID"])
			u.Restarts, _ = strconv.Atoi(cur["NRestarts"])
			// "[not set]" or 2^64-1 when there's no memory accounting.
			if m, err := strconv.ParseInt(cur["MemoryCurrent"], 10, 64); err == nil && m > 0 {
				u.Memory = m
			}
			if us, err := strconv.ParseInt(cur["ActiveEnterTimestampMonotonic"], 10, 64); err == nil && us > 0 && !boot.IsZero() && u.Active == "active" {
				u.Since = boot.Add(time.Duration(us) * time.Microsecond).Truncate(time.Second)
			}
			units = append(units, u)
		}
		cur = nil
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if cur == nil {
			cur = map[string]string{}
		}
		cur[k] = v
	}
	flush()
	return units
}
