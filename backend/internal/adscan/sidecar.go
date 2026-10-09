package adscan

// Enigma2's recording sidecars as expected from the open Enigma2 sources;
// spikes/adscan/phase0.sh confirms the layout on DreamOS before anything
// writes a cut list.
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

type AccessPoint struct {
	Offset int64 // byte offset of the keyframe in the .ts
	PTS    int64 // unwrapped, so it only grows
}

// ParseAP reads a .ts.ap. Timestamps are unwrapped so they grow
// monotonically across a 33-bit wrap.
func ParseAP(data []byte) ([]AccessPoint, error) {
	if len(data)%16 != 0 {
		return nil, fmt.Errorf("ap: %d bytes is not a multiple of 16", len(data))
	}
	aps := make([]AccessPoint, 0, len(data)/16)
	var wraps, last int64 = 0, -1
	for i := 0; i < len(data); i += 16 {
		off := int64(binary.BigEndian.Uint64(data[i:]))
		pts := int64(binary.BigEndian.Uint64(data[i+8:])) & (ptsWrap - 1)
		if last >= 0 && pts < last && last-pts > ptsWrap/2 {
			wraps++
		}
		last = pts
		aps = append(aps, AccessPoint{Offset: off, PTS: pts + wraps*ptsWrap})
	}
	return aps, nil
}

// APAt returns the last access point at or before sec seconds into the
// recording (counted from the first access point), so decoding from its
// offset yields the keyframe for that moment.
func APAt(aps []AccessPoint, sec float64) (AccessPoint, bool) {
	if len(aps) == 0 {
		return AccessPoint{}, false
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
	CutIn   = 0
	CutOut  = 1
	CutMark = 2
	CutLast = 3 // resume position
)

type Cut struct {
	PTS  uint64
	Type uint32
}

func ParseCuts(data []byte) ([]Cut, error) {
	if len(data)%12 != 0 {
		return nil, fmt.Errorf("cuts: %d bytes is not a multiple of 12", len(data))
	}
	cuts := make([]Cut, 0, len(data)/12)
	for i := 0; i < len(data); i += 12 {
		cuts = append(cuts, Cut{
			PTS:  binary.BigEndian.Uint64(data[i:]),
			Type: binary.BigEndian.Uint32(data[i+8:]),
		})
	}
	return cuts, nil
}

func EncodeCuts(cuts []Cut) []byte {
	out := make([]byte, 12*len(cuts))
	for i, c := range cuts {
		binary.BigEndian.PutUint64(out[12*i:], c.PTS)
		binary.BigEndian.PutUint32(out[12*i+8:], c.Type)
	}
	return out
}

// PTSSpan is a stretch of a recording in stream PTS (90 kHz).
type PTSSpan struct{ Start, End uint64 }

// CutRegions turns a list's IN/OUT entries into the regions the player
// skips. Enigma2's rule: OUT starts a cut region, IN ends it; an IN before
// any OUT means everything before it is cut; an OUT without a following IN
// cuts to the end.
func CutRegions(cuts []Cut) []PTSSpan {
	var sorted []Cut
	for _, c := range cuts {
		if c.Type == CutIn || c.Type == CutOut {
			sorted = append(sorted, c)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PTS < sorted[j].PTS })
	var regions []PTSSpan
	var open bool
	var start uint64
	for i, c := range sorted {
		switch {
		case c.Type == CutOut && !open:
			open, start = true, c.PTS
		case c.Type == CutIn && open:
			open = false
			regions = append(regions, PTSSpan{start, c.PTS})
		case c.Type == CutIn && i == 0:
			regions = append(regions, PTSSpan{0, c.PTS})
		}
	}
	if open {
		regions = append(regions, PTSSpan{start, ^uint64(0)})
	}
	return regions
}

// MergeAds adds ad blocks to an existing cut list. Marks and the resume
// position stay untouched; the existing cut regions (perhaps set by hand in
// CutListEditor) and the new blocks are united, so no cut the user made is
// lost, and overlapping ones become one region.
func MergeAds(existing []Cut, blocks []PTSSpan) []Cut {
	var out []Cut
	for _, c := range existing {
		if c.Type != CutIn && c.Type != CutOut {
			out = append(out, c)
		}
	}
	regions := append(CutRegions(existing), blocks...)
	sort.Slice(regions, func(i, j int) bool { return regions[i].Start < regions[j].Start })
	var merged []PTSSpan
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
			out = append(out, Cut{r.Start, CutOut})
		}
		if r.End != ^uint64(0) {
			out = append(out, Cut{r.End, CutIn})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PTS < out[j].PTS })
	return out
}

// WriteCutsAtomic replaces path through a temp file in the same folder, so
// a crash leaves either the old or the new list, never half of one.
func WriteCutsAtomic(path string, cuts []Cut) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cuts-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(EncodeCuts(cuts)); err != nil {
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
