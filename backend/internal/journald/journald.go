// Package journald reads systemd's journal for AdminHub (docs/archive/V1_PLAN.md,
// WP6). The journal on the reference box is volatile — RAM-backed, lost on
// every reboot (ARCHITECTURE.md, "journald is volatile") — so a Reader only
// ever promises "since last boot", never real history.
package journald

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Level is a syslog priority, 0 (emerg) through 7 (debug) — journalctl's own
// scale, most severe first.
type Level int

const (
	LevelEmerg Level = iota
	LevelAlert
	LevelCrit
	LevelErr
	LevelWarning
	LevelNotice
	LevelInfo
	LevelDebug
)

var levelNames = [...]string{"emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"}

func (l Level) String() string {
	if l < 0 || int(l) >= len(levelNames) {
		return "info"
	}
	return levelNames[l]
}

// Entry is one journal line, trimmed to what AdminHub's UI shows.
type Entry struct {
	Time    time.Time `json:"time"`
	Unit    string    `json:"unit,omitempty"`
	Level   Level     `json:"level"`
	Message string    `json:"message"`
}

// Query narrows a read. Zero values mean "no filter" except Limit, which
// callers should always set — journalctl with no bound can return the
// journal's entire RAM buffer.
type Query struct {
	Unit     string // "" = every unit
	MaxLevel Level  // include entries at this severity or worse (lower number); LevelDebug (7) means every level
	Text     string // case-insensitive substring match on Message
	Limit    int
}

// Reader reads journal entries, most recent last (the order a log viewer
// scrolls to).
type Reader interface {
	Read(ctx context.Context, q Query) ([]Entry, error)
}

// ErrUnavailable means journalctl isn't on this system — the same
// capability check AdminHub's own status already makes (hub.CapJournald),
// surfaced here too for direct callers.
var ErrUnavailable = errors.New("journald: journalctl not available")

// Exec reads the real journal by shelling out to journalctl, the only way
// to get at it (no Go client library, and reading /run/log/journal's binary
// format directly would mean reimplementing systemd's own reader).
type Exec struct{}

func (Exec) Read(ctx context.Context, q Query) ([]Entry, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	args := []string{"-o", "json", "--no-pager", "-n", strconv.Itoa(limit)}
	if q.Unit != "" {
		args = append(args, "-u", q.Unit)
	}
	if q.MaxLevel > 0 && q.MaxLevel < LevelDebug {
		args = append(args, "-p", "0.."+strconv.Itoa(int(q.MaxLevel)))
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.Error
		if errors.As(err, &ee) {
			return nil, ErrUnavailable
		}
		return nil, err
	}
	return parse(out, q.Text), nil
}

// rawEntry mirrors journalctl -o json's field names (all strings, journald's
// own convention — even the numeric ones).
type rawEntry struct {
	Timestamp string `json:"__REALTIME_TIMESTAMP"`
	Unit      string `json:"_SYSTEMD_UNIT"`
	Priority  string `json:"PRIORITY"`
	Message   any    `json:"MESSAGE"` // usually a string; journald sends non-UTF-8 messages as a byte array instead
}

func parse(out []byte, textFilter string) []Entry {
	textFilter = strings.ToLower(textFilter)
	var entries []Entry
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var raw rawEntry
		if err := json.Unmarshal(sc.Bytes(), &raw); err != nil {
			continue
		}
		msg, ok := raw.Message.(string)
		if !ok {
			msg = "" // non-UTF-8 message (byte array form) — not shown rather than mangled
		}
		if textFilter != "" && !strings.Contains(strings.ToLower(msg), textFilter) {
			continue
		}
		microsec, _ := strconv.ParseInt(raw.Timestamp, 10, 64)
		prio, err := strconv.Atoi(raw.Priority)
		if err != nil || prio < 0 || prio > int(LevelDebug) {
			prio = int(LevelInfo)
		}
		entries = append(entries, Entry{
			Time:    time.UnixMicro(microsec).UTC(),
			Unit:    raw.Unit,
			Level:   Level(prio),
			Message: msg,
		})
	}
	return entries
}
