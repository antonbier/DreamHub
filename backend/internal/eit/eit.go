// Package eit reads the programme data Enigma2 keeps next to a recording
// (<name>.eit): the first event of a DVB EIT section, without the section
// header — event ID, start (modified Julian date and BCD time, UTC) and
// duration (BCD). Seen on the reference box 2026-10-20 (52 files, all
// starting that way). DreamHub uses it to compare what a recording was
// scheduled to be with what was recorded (MediaHub, "cut short").
package eit

import (
	"errors"
	"time"
)

// Event is the programme a recording was made of, as the broadcaster
// announced it.
type Event struct {
	ID       uint16
	Start    time.Time
	Duration time.Duration
}

// ErrShort: not enough bytes for an event.
var ErrShort = errors.New("eit: too short")

// ErrInvalid: the bytes aren't a plausible event.
var ErrInvalid = errors.New("eit: not an event")

// Parse reads the first event of an .eit file.
func Parse(b []byte) (Event, error) {
	if len(b) < 10 {
		return Event{}, ErrShort
	}
	mjd := int(b[2])<<8 | int(b[3])
	h, okH := bcd(b[4])
	m, okM := bcd(b[5])
	s, okS := bcd(b[6])
	dh, okDH := bcd(b[7])
	dm, okDM := bcd(b[8])
	ds, okDS := bcd(b[9])
	if !(okH && okM && okS && okDH && okDM && okDS) || h > 23 || m > 59 || s > 59 || dm > 59 || ds > 59 {
		return Event{}, ErrInvalid
	}
	// Modified Julian date → day (EN 300 468, annex C).
	day := time.Date(1858, time.November, 17, 0, 0, 0, 0, time.UTC).AddDate(0, 0, mjd)
	return Event{
		ID:       uint16(b[0])<<8 | uint16(b[1]),
		Start:    day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second),
		Duration: time.Duration(dh)*time.Hour + time.Duration(dm)*time.Minute + time.Duration(ds)*time.Second,
	}, nil
}

func bcd(b byte) (int, bool) {
	hi, lo := int(b>>4), int(b&15)
	return hi*10 + lo, hi <= 9 && lo <= 9
}
