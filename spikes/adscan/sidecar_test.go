package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func apBytes(entries ...[2]uint64) []byte {
	out := make([]byte, 16*len(entries))
	for i, e := range entries {
		binary.BigEndian.PutUint64(out[16*i:], e[0])
		binary.BigEndian.PutUint64(out[16*i+8:], e[1])
	}
	return out
}

func TestParseAPUnwrapsPTS(t *testing.T) {
	// Starts 1 s before the 33-bit wrap, one access point per second.
	start := uint64(ptsWrap - ptsHz)
	data := apBytes(
		[2]uint64{0, start},
		[2]uint64{188_000, (start + ptsHz) % ptsWrap},
		[2]uint64{376_000, (start + 2*ptsHz) % ptsWrap},
	)
	aps, err := parseAP(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(aps); i++ {
		if aps[i].PTS-aps[i-1].PTS != ptsHz {
			t.Fatalf("entry %d: PTS step %d, want %d", i, aps[i].PTS-aps[i-1].PTS, ptsHz)
		}
	}
	if ap, _ := at(aps, 1.5); ap.Offset != 188_000 {
		t.Fatalf("at(1.5 s) = offset %d, want 188000", ap.Offset)
	}
	if ap, _ := at(aps, 99); ap.Offset != 376_000 {
		t.Fatalf("at(99 s) = offset %d, want the last access point", ap.Offset)
	}
}

func TestParseAPRejectsTornFile(t *testing.T) {
	if _, err := parseAP(make([]byte, 20)); err == nil {
		t.Fatal("want an error for a size that is not a multiple of 16")
	}
}

func TestCutsRoundTrip(t *testing.T) {
	in := []cut{{900_000, cutMark}, {1_800_000, cutLast}}
	out, err := parseCuts(encodeCuts(in))
	if err != nil || !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip: %v %v", out, err)
	}
}

func TestMergeAdsKeepsMarksResumeAndUserCuts(t *testing.T) {
	existing := []cut{
		{100, cutIn},    // the user skipped the pre-margin
		{500, cutMark},  // a mark
		{9_000, cutOut}, // the user's own cut, overlapping an ad block
		{10_000, cutIn},
		{20_000, cutLast}, // resume position
	}
	got := mergeAds(existing, []block{{9_500, 12_000}, {15_000, 16_000}})
	want := []cut{
		{100, cutIn},
		{500, cutMark},
		{9_000, cutOut}, {12_000, cutIn}, // united with the user's cut
		{15_000, cutOut}, {16_000, cutIn},
		{20_000, cutLast},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestMergeAdsOpenEnd(t *testing.T) {
	got := mergeAds([]cut{{5_000, cutOut}}, []block{{1_000, 2_000}})
	want := []cut{{1_000, cutOut}, {2_000, cutIn}, {5_000, cutOut}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWriteCutsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rec.ts.cuts")
	if err := os.WriteFile(path, encodeCuts([]cut{{1, cutLast}}), 0o644); err != nil {
		t.Fatal(err)
	}
	want := []cut{{1, cutLast}, {2, cutOut}, {3, cutIn}}
	if err := writeCutsAtomic(path, want); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got, _ := parseCuts(data)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}
