package hubs

import (
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/ai"
)

func TestCandidates_FairAcrossChannelsEveningFirst(t *testing.T) {
	loc := time.FixedZone("CEST", 2*3600)
	day := time.Date(2026, 10, 6, 0, 0, 0, 0, loc)
	var items []ai.EPGItem
	// The box lists channel by channel: four channels, one show per hour
	// from 06:00 to 23:00 each.
	for _, ch := range []string{"a", "b", "c", "d"} {
		for h := 6; h < 24; h++ {
			items = append(items, ai.EPGItem{ServiceReference: ch, Start: day.Add(time.Duration(h) * time.Hour).Unix()})
		}
	}
	out := candidates(items, 20, loc)
	if len(out) != 20 {
		t.Fatalf("got %d", len(out))
	}
	per := map[string]int{}
	for i, it := range out {
		per[it.ServiceReference]++
		if h := time.Unix(it.Start, 0).In(loc).Hour(); h < 18 {
			t.Errorf("evening first: %02d:00 on %s", h, it.ServiceReference)
		}
		if i > 0 && out[i-1].Start > it.Start {
			t.Error("not in time order")
		}
	}
	for _, ch := range []string{"a", "b", "c", "d"} {
		if per[ch] != 5 {
			t.Errorf("every channel gets its share: %v", per)
		}
	}
	if got := candidates(items[:3], 20, loc); len(got) != 3 {
		t.Error("under the cap nothing changes")
	}
}
