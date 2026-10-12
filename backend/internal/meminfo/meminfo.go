// Package meminfo reads the system's free memory for AppsHub's
// install-time resource budget check (docs/APPSHUB_PLAN.md, App4:
// "Resource budget: computed live, not a hardcoded number"). Like
// internal/systemd and internal/journald: Exec reads the real
// /proc/meminfo, ./mock answers from memory.
//
// MemAvailable (not MemTotal minus a guessed reservation) is used
// directly: the kernel already nets out what enigma2, DreamHub and the
// rest of the OS currently hold, including reclaimable cache — exactly
// the "what's actually free right now" the plan asks for, with no need
// to separately read every other process's own memory use.
package meminfo

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
)

// Info is the system's memory state, in bytes.
type Info struct {
	TotalBytes     int64
	AvailableBytes int64
}

// Reader reads the current memory state.
type Reader interface {
	Read(ctx context.Context) (Info, error)
}

// ErrUnavailable means /proc/meminfo couldn't be read (not Linux, or a
// restricted sandbox).
var ErrUnavailable = errors.New("meminfo: /proc/meminfo not available")

// Exec reads /proc/meminfo.
type Exec struct{}

func (Exec) Read(ctx context.Context) (Info, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return Info{}, ErrUnavailable
	}
	defer f.Close()

	var info Info
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			info.TotalBytes = kb * 1024
		case "MemAvailable":
			info.AvailableBytes = kb * 1024
		}
	}
	if err := sc.Err(); err != nil {
		return Info{}, err
	}
	return info, nil
}
