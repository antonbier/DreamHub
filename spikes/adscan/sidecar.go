package main

// Enigma2's recording sidecars as expected from the open Enigma2 sources;
// phase0.sh confirms the layout on DreamOS before anything relies on it.
//
//	.ts.ap   16-byte entries: 64-bit BE byte offset, 64-bit BE PTS (90 kHz)
//	.ts.cuts 12-byte entries: 64-bit BE PTS, 32-bit BE type

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// The PTS is a 33-bit counter at 90 kHz; it wraps after about 26.5 hours,
// and a recording can start just before the wrap.
const (
	ptsHz   = 90000
	ptsWrap = 1 << 33
)

type accessPoint struct {
	Offset int64 // byte offset of the keyframe in the .ts
	PTS    int64 // unwrapped, so it only grows
}

// parseAP reads a .ts.ap. Timestamps are unwrapped so they grow
// monotonically across a 33-bit wrap.
func parseAP(data []byte) ([]accessPoint, error) {
	if len(data)%16 != 0 {
		return nil, fmt.Errorf("ap: %d bytes is not a multiple of 16", len(data))
	}
	aps := make([]accessPoint, 0, len(data)/16)
	var wraps, last int64 = 0, -1
	for i := 0; i < len(data); i += 16 {
		off := int64(binary.BigEndian.Uint64(data[i:]))
		pts := int64(binary.BigEndian.Uint64(data[i+8:])) & (ptsWrap - 1)
		if last >= 0 && pts < last && last-pts > ptsWrap/2 {
			wraps++
		}
		last = pts
		aps = append(aps, accessPoint{Offset: off, PTS: pts + wraps*ptsWrap})
	}
	return aps, nil
}

// at returns the last access point at or before sec seconds into the
// recording (counted from the first access point), so decoding from its
// offset yields the keyframe for that moment.
func at(aps []accessPoint, sec float64) (accessPoint, bool) {
	if len(aps) == 0 {
		return accessPoint{}, false
	}
	want := aps[0].PTS + int64(sec*ptsHz)
	i := sort.Search(len(aps), func(i int) bool { return aps[i].PTS > want })
	if i == 0 {
		return aps[0], true
	}
	return aps[i-1], true
}

// Cut list entry types.
const (
	cutIn   = 0
	cutOut  = 1
	cutMark = 2
	cutLast = 3 // resume position
)

type cut struct {
	PTS  uint64
	Type uint32
}

func parseCuts(data []byte) ([]cut, error) {
	if len(data)%12 != 0 {
		return nil, fmt.Errorf("cuts: %d bytes is not a multiple of 12", len(data))
	}
	cuts := make([]cut, 0, len(data)/12)
	for i := 0; i < len(data); i += 12 {
		cuts = append(cuts, cut{
			PTS:  binary.BigEndian.Uint64(data[i:]),
			Type: binary.BigEndian.Uint32(data[i+8:]),
		})
	}
	return cuts, nil
}

func encodeCuts(cuts []cut) []byte {
	out := make([]byte, 12*len(cuts))
	for i, c := range cuts {
		binary.BigEndian.PutUint64(out[12*i:], c.PTS)
		binary.BigEndian.PutUint32(out[12*i+8:], c.Type)
	}
	return out
}

// block is a confirmed ad block, in stream PTS.
type block struct{ Start, End uint64 }

// cutRegions turns a list's IN/OUT entries into the regions the player
// skips. Enigma2's rule: OUT starts a cut region, IN ends it; an IN before
// any OUT means everything before it is cut; an OUT without a following IN
// cuts to the end.
func cutRegions(cuts []cut) []block {
	var sorted []cut
	for _, c := range cuts {
		if c.Type == cutIn || c.Type == cutOut {
			sorted = append(sorted, c)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PTS < sorted[j].PTS })
	var regions []block
	var open bool
	var start uint64
	for i, c := range sorted {
		switch {
		case c.Type == cutOut && !open:
			open, start = true, c.PTS
		case c.Type == cutIn && open:
			open = false
			regions = append(regions, block{start, c.PTS})
		case c.Type == cutIn && i == 0:
			regions = append(regions, block{0, c.PTS})
		}
	}
	if open {
		regions = append(regions, block{start, ^uint64(0)})
	}
	return regions
}

// mergeAds adds ad blocks to an existing cut list. Marks and the resume
// position stay untouched; the existing cut regions (perhaps set by hand in
// CutListEditor) and the new blocks are united, so no cut the user made is
// lost, and overlapping ones become one region.
func mergeAds(existing []cut, blocks []block) []cut {
	var out []cut
	for _, c := range existing {
		if c.Type != cutIn && c.Type != cutOut {
			out = append(out, c)
		}
	}
	regions := append(cutRegions(existing), blocks...)
	sort.Slice(regions, func(i, j int) bool { return regions[i].Start < regions[j].Start })
	var merged []block
	for _, r := range regions {
		if n := len(merged); n > 0 && r.Start <= merged[n-1].End {
			merged[n-1].End = max(merged[n-1].End, r.End)
			continue
		}
		merged = append(merged, r)
	}
	for _, r := range merged {
		// A region from the very beginning is a lone IN, the way Enigma2
		// writes "skip the start"; one to the end is a lone OUT.
		if r.Start > 0 {
			out = append(out, cut{r.Start, cutOut})
		}
		if r.End != ^uint64(0) {
			out = append(out, cut{r.End, cutIn})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PTS < out[j].PTS })
	return out
}

// writeCutsAtomic replaces path through a temp file in the same folder, so
// a crash leaves either the old or the new list, never half of one.
func writeCutsAtomic(path string, cuts []cut) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cuts-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encodeCuts(cuts)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
