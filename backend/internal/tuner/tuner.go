// Package tuner estimates whether a set of recordings fits the box's tuners.
//
// It is an estimate, not the truth: the box itself decides on every
// /web/timeradd by simulating a real tuner allocation (TimerSanityCheck —
// see ARCHITECTURE.md), which also knows about FBC inputs, Unicable and
// LNB setups DreamHub can't see. This package only has to be good enough
// for a preview — better than "number of timers vs. number of tuners",
// which is wrong both ways: one tuner records every channel of a
// transponder at once, and a cable tuner can't record a satellite channel.
// See docs/V1_PLAN.md, "Tuners".
package tuner

import (
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Delivery is a delivery system: what kind of signal a channel arrives on.
type Delivery int

const (
	Unknown Delivery = iota
	Satellite
	Cable
	Terrestrial
)

func (d Delivery) String() string {
	switch d {
	case Satellite:
		return "satellite"
	case Cable:
		return "cable"
	case Terrestrial:
		return "terrestrial"
	}
	return "unknown"
}

// Transponder identifies what one tuner has to be tuned to. Every channel on
// the same transponder can be recorded with one tuner.
type Transponder struct {
	TSID, ONID, Namespace uint64
}

// Service is the part of an Enigma2 service reference that matters here.
type Service struct {
	SID         uint64
	Transponder Transponder
}

// Delivery derives the delivery system from the namespace: Enigma2 uses
// 0xFFFF…. for cable and 0xEEEE…. for terrestrial; anything else encodes a
// satellite's orbital position.
func (s Service) Delivery() Delivery {
	switch s.Transponder.Namespace >> 16 {
	case 0xFFFF:
		return Cable
	case 0xEEEE:
		return Terrestrial
	}
	return Satellite
}

// ParseService reads a DVB service reference such as
// "1:0:19:2EE3:441:1:C00000:0:0:0:" (type:flags:stype:SID:TSID:ONID:NS:…).
// ok is false for anything that doesn't need a tuner or can't be parsed —
// recordings being played back, IPTV streams (which carry a path/URL in
// field 11) and malformed references.
func ParseService(ref string) (s Service, ok bool) {
	f := strings.Split(ref, ":")
	if len(f) < 10 || f[0] != "1" {
		return Service{}, false
	}
	if len(f) > 10 && strings.TrimSpace(strings.Join(f[10:], ":")) != "" {
		return Service{}, false
	}
	var v [4]uint64
	for i := range v {
		n, err := strconv.ParseUint(f[3+i], 16, 64)
		if err != nil {
			return Service{}, false
		}
		v[i] = n
	}
	s = Service{SID: v[0], Transponder: Transponder{TSID: v[1], ONID: v[2], Namespace: v[3]}}
	if s.Transponder == (Transponder{}) {
		return Service{}, false
	}
	return s, true
}

// SameService reports whether two references name the same channel — the
// comparison the box's own duplicate check makes (SID, TSID, ONID,
// namespace), independent of flags or service type.
func SameService(a, b string) bool {
	sa, oka := ParseService(a)
	sb, okb := ParseService(b)
	if !oka || !okb {
		return a == b
	}
	return sa == sb
}

// Frontend is one tuner and the delivery systems it can receive.
type Frontend struct {
	Name     string
	Supports map[Delivery]bool
}

func (f Frontend) can(d Delivery) bool { return d == Unknown || f.Supports[d] }

// Frontends derives the tuners from /web/deviceinfo, reading each model
// string's capabilities, e.g. "Si2169D (DVB-S2 / DVB-C / DVB-T2)". A model
// naming no delivery system is assumed to receive everything — too lax is
// the safer mistake for an estimate the box double-checks. No frontends at
// all means one universal tuner (the reference box's case, and the old
// default).
func Frontends(info *webcontrol.DeviceInfo) []Frontend {
	if info == nil || len(info.Frontends) == 0 {
		return []Frontend{{Name: "Tuner", Supports: all()}}
	}
	out := make([]Frontend, 0, len(info.Frontends))
	for _, fe := range info.Frontends {
		m := strings.ToUpper(fe.Model)
		sup := map[Delivery]bool{}
		if strings.Contains(m, "DVB-S") {
			sup[Satellite] = true
		}
		if strings.Contains(m, "DVB-C") {
			sup[Cable] = true
		}
		if strings.Contains(m, "DVB-T") {
			sup[Terrestrial] = true
		}
		if len(sup) == 0 {
			sup = all()
		}
		out = append(out, Frontend{Name: fe.Name, Supports: sup})
	}
	return out
}

func all() map[Delivery]bool {
	return map[Delivery]bool{Satellite: true, Cable: true, Terrestrial: true}
}

// Booking is a recording occupying a tuner from Begin to End (Unix seconds).
type Booking struct {
	ServiceReference string
	Begin, End       int64
}

// Conflicts returns the indices of the existing bookings that, together
// with candidate, likely need more tuners than there are — nil if the
// candidate is expected to fit. It checks every instant where the set of
// running recordings changes during the candidate's window; at each one,
// the transponders in use must be assignable to distinct compatible tuners.
func Conflicts(frontends []Frontend, existing []Booking, candidate Booking) []int {
	if _, ok := ParseService(candidate.ServiceReference); !ok {
		return nil // doesn't need a tuner
	}
	var overlapping []int
	instants := []int64{candidate.Begin}
	for i, b := range existing {
		if b.Begin < candidate.End && candidate.Begin < b.End {
			overlapping = append(overlapping, i)
			if b.Begin > candidate.Begin {
				instants = append(instants, b.Begin)
			}
		}
	}

	clash := map[int]bool{}
	for _, t := range instants {
		var running []int
		for _, i := range overlapping {
			if existing[i].Begin <= t && t < existing[i].End {
				running = append(running, i)
			}
		}
		refs := []string{candidate.ServiceReference}
		for _, i := range running {
			refs = append(refs, existing[i].ServiceReference)
		}
		if !fits(frontends, refs) {
			for _, i := range running {
				clash[i] = true
			}
		}
	}
	if len(clash) == 0 {
		return nil
	}
	out := make([]int, 0, len(clash))
	for _, i := range overlapping {
		if clash[i] {
			out = append(out, i)
		}
	}
	return out
}

// fits reports whether every transponder needed by refs can get its own
// compatible tuner — a small bipartite matching (augmenting paths).
func fits(frontends []Frontend, refs []string) bool {
	type need struct {
		tp       Transponder
		delivery Delivery
	}
	seen := map[Transponder]bool{}
	var needs []need
	for _, r := range refs {
		s, ok := ParseService(r)
		if !ok {
			continue
		}
		if !seen[s.Transponder] {
			seen[s.Transponder] = true
			needs = append(needs, need{s.Transponder, s.Delivery()})
		}
	}
	if len(needs) > len(frontends) {
		return false
	}
	owner := make([]int, len(frontends)) // frontend -> index into needs, or -1
	for i := range owner {
		owner[i] = -1
	}
	var try func(n int, visited []bool) bool
	try = func(n int, visited []bool) bool {
		for f := range frontends {
			if visited[f] || !frontends[f].can(needs[n].delivery) {
				continue
			}
			visited[f] = true
			if owner[f] == -1 || try(owner[f], visited) {
				owner[f] = n
				return true
			}
		}
		return false
	}
	for n := range needs {
		if !try(n, make([]bool, len(frontends))) {
			return false
		}
	}
	return true
}
