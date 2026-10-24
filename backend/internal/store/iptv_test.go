package store

import (
	"context"
	"errors"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/box"
)

func TestIPTVSources(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	src := IPTVSource{Name: "tv.example", URL: "http://u:p@tv.example/list.m3u", DisplayURL: "http://tv.example/list.m3u",
		BouquetRef: "1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"userbouquet.internet.tv\" ORDER BY bouquet", BouquetName: "Internet (TV)", Kind: "tv",
		Seen: []uint64{9, 3, 3, 7}}
	id, err := s.SaveIPTVSource(ctx, src, []IPTVMember{{"h1", "i1", "One"}, {"h2", "i2", "Two"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.IPTVSource(ctx, id)
	if err != nil || got.URL != src.URL || len(got.Seen) != 3 || got.Seen[0] != 3 || got.Seen[2] != 9 || got.ComparedAt.IsZero() {
		t.Fatalf("source: %+v %v", got, err)
	}
	// the listing never carries the address or the keys
	list, _ := s.IPTVSources(ctx)
	if len(list) != 1 || list[0].URL != "" || list[0].Seen != nil || list[0].DisplayURL != src.DisplayURL {
		t.Fatalf("list: %+v", list)
	}
	// the same list into the same bouquet again is the same source
	id2, err := s.SaveIPTVSource(ctx, src, []IPTVMember{{"h3", "i3", "Three"}})
	if err != nil || id2 != id {
		t.Fatalf("second save: %d %v", id2, err)
	}
	if m, _ := s.IPTVMembers(ctx, id); len(m) != 3 {
		t.Fatalf("members: %+v", m)
	}
	// a change: one member leaves, one comes, the bouquet was renamed
	if err := s.UpdateIPTVSource(ctx, id, IPTVChange{RemoveHashes: []string{"h1"}, Add: []IPTVMember{{"h4", "i1", "One"}}, Seen: []uint64{1}, BouquetName: "Net"}); err != nil {
		t.Fatal(err)
	}
	m, _ := s.IPTVMembers(ctx, id)
	hashes := map[string]bool{}
	for _, x := range m {
		hashes[x.URLHash] = true
	}
	if hashes["h1"] || !hashes["h4"] || len(m) != 3 {
		t.Fatalf("members after: %+v", m)
	}
	got, _ = s.IPTVSource(ctx, id)
	if got.BouquetName != "Net" || len(got.Seen) != 1 {
		t.Fatalf("after: %+v", got)
	}

	// another box does not see it
	other := box.With(ctx, "second")
	if l, _ := s.IPTVSources(other); len(l) != 0 {
		t.Fatalf("another box sees %+v", l)
	}
	if _, err := s.IPTVSource(other, id); !errors.Is(err, ErrIPTVSourceNotFound) {
		t.Fatalf("another box reads it: %v", err)
	}
	if err := s.DeleteIPTVSource(other, id); !errors.Is(err, ErrIPTVSourceNotFound) {
		t.Fatalf("another box deletes it: %v", err)
	}
	// forgetting takes the members along
	if err := s.DeleteIPTVSource(ctx, id); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.IPTVMembers(ctx, id); len(m) != 0 {
		t.Fatalf("members remain: %+v", m)
	}
}

func TestIPTVTwins(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	if err := s.SaveIPTVTwins(ctx, map[string]IPTVTwin{"a": {"1:0:19:283D:3FB:1:C00000:0:0:0:", "Das Erste HD"}, "b": {"1:0:1:1:1:1:C00000:0:0:0:", "ZDF"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveIPTVTwins(ctx, map[string]IPTVTwin{"a": {"1:0:19:2:2:1:C00000:0:0:0:", "Das Erste"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.IPTVTwins(ctx)
	if len(got) != 2 || got["a"].ServiceName != "Das Erste" {
		t.Fatalf("twins: %+v", got)
	}
	if other, _ := s.IPTVTwins(box.With(ctx, "second")); len(other) != 0 {
		t.Fatalf("another box sees %+v", other)
	}
	s.ForgetIPTVTwin(ctx, "a")
	if got, _ = s.IPTVTwins(ctx); len(got) != 1 {
		t.Fatalf("after forgetting: %+v", got)
	}
}
