package hub

import (
	"testing"
	"time"
)

func TestApplyView(t *testing.T) {
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	day := int64(86400)
	rome, _ := time.LoadLocation("Europe/Rome")
	items := []ViewItem{
		{Title: "Angel Has Fallen", Subtitle: "ORF1 HD", Time: now.Unix() - 400*day, Size: 4_400_000_000},
		{Title: "Yesterday", Subtitle: "VOX", Time: now.Unix() - 200*day, Size: 3_800_000_000},
		{Title: "Tatort", Subtitle: "Das Erste HD", Time: now.Unix() - 2*day, Size: 5_000_000_000},
		{Title: "Kurz", Subtitle: "ZDF", Time: now.Unix() - 100*day, Size: 500_000_000},
	}
	recordings := ViewSource{Filters: []string{FilterText, FilterOlderThanDays, FilterMinSizeGB}, Sorts: []string{SortNewest, SortLargest}}

	res := applyView(items, recordings, ViewQuery{OlderThanDays: 90, MinSizeGB: 3, Sort: SortLargest}, now, nil)
	if res.Total != 2 || res.Items[0].Title != "Angel Has Fallen" || res.TotalSize != 8_200_000_000 {
		t.Errorf("old and large, by size: %+v", res)
	}

	res = applyView(items, recordings, ViewQuery{Text: "das erste"}, now, nil)
	if res.Total != 1 || res.Items[0].Title != "Tatort" {
		t.Errorf("text matches the subtitle too: %+v", res)
	}

	// A filter the source doesn't offer is ignored, and an unknown sort
	// falls back to the source's first.
	res = applyView(items, recordings, ViewQuery{WarnOnly: true, Sort: "bogus", Limit: 2}, now, nil)
	if res.Total != 4 || len(res.Items) != 2 || res.Items[0].Title != "Tatort" {
		t.Errorf("unsupported filter ignored, newest first, limited: %+v", res)
	}

	guide := ViewSource{Filters: []string{FilterFromTime, FilterMinDuration, FilterWithinDays}, Sorts: []string{SortSoonest}}
	at := func(h, m int, d time.Duration) (int64, int64) {
		s := time.Date(2026, 9, 23, h, m, 0, 0, rome)
		return s.Unix(), s.Add(d).Unix()
	}
	s1, e1 := at(19, 30, 30*time.Minute)
	s2, e2 := at(20, 15, 100*time.Minute)
	s3, e3 := at(22, 0, 45*time.Minute)
	s4, e4 := time.Date(2026, 9, 26, 20, 15, 0, 0, rome).Unix(), time.Date(2026, 9, 26, 22, 0, 0, 0, rome).Unix()
	evening := []ViewItem{{Title: "c", Time: s3, End: e3}, {Title: "a", Time: s1, End: e1}, {Title: "b", Time: s2, End: e2}, {Title: "later", Time: s4, End: e4}}
	res = applyView(evening, guide, ViewQuery{FromTime: "20:15", MinDuration: 80, WithinDays: 1}, now, rome)
	if res.Total != 1 || res.Items[0].Title != "b" {
		t.Errorf("movies tonight from 20:15 (in the display zone): %+v", res)
	}
	res = applyView(evening, guide, ViewQuery{FromTime: "20:15"}, now, rome)
	if res.Total != 3 || res.Items[0].Title != "b" || res.Items[1].Title != "c" {
		t.Errorf("fromTime applies on any day, soonest first: %+v", res)
	}
}
