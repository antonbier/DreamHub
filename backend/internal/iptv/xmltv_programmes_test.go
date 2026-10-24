package iptv

import (
	"strings"
	"testing"
	"time"
)

const scheduleXML = `<?xml version="1.0"?><tv>
<channel id="a.de"><display-name>Das Erste HD</display-name></channel>
<channel id="b.de"><display-name>Pluto News</display-name></channel>
<programme start="20261024180000 +0200" stop="20261024190000 +0200" channel="a.de"><title>Vorher</title></programme>
<programme start="20261024190000 +0200" stop="20261024200000 +0200" channel="a.de"><title>Jetzt &amp; hier</title></programme>
<programme start="20261024200000 +0200" stop="20261024210000 +0200" channel="a.de"><title>Danach</title></programme>
<programme start="20261023100000 +0200" stop="20261023110000 +0200" channel="a.de"><title>Gestern</title></programme>
<programme start="20261024190000 +0000" stop="20261024200000 +0000" channel="b.de"><title>Nachrichten</title></programme>
</tv>`

func TestSchedule_NowNextByIDAndByName(t *testing.T) {
	at := time.Date(2026, 10, 24, 17, 30, 0, 0, time.UTC) // 19:30 local (+0200)
	s, err := ParseSchedule(strings.NewReader(scheduleXML), at.Add(-time.Hour), at.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.ByID["a.de"]); n != 3 {
		t.Fatalf("a.de keeps the window only, got %d", n)
	}
	now, next := s.Find("a.de", "", at)
	if now == nil || now.Title != "Jetzt & hier" || next == nil || next.Title != "Danach" {
		t.Fatalf("by id: %+v %+v", now, next)
	}
	// no matching id: found by name, "HD" ignored
	now, _ = s.Find("other", "Das Erste", at)
	if now == nil || now.Title != "Jetzt & hier" {
		t.Fatalf("by name: %+v", now)
	}
	if now, next = s.Find("zzz", "unknown", at); now != nil || next != nil {
		t.Fatalf("unknown channel: %+v %+v", now, next)
	}
	// nothing running yet: only the next one
	early := time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC)
	s2, _ := ParseSchedule(strings.NewReader(scheduleXML), early.Add(-time.Hour), early.Add(6*time.Hour))
	if now, next = s2.Find("b.de", "", early); now != nil || next == nil || next.Title != "Nachrichten" {
		t.Fatalf("gap: %+v %+v", now, next)
	}
	if _, err := ParseSchedule(strings.NewReader("<html></html>"), at, at); err == nil {
		t.Error("an HTML page is not a guide")
	}
}

func TestNormName(t *testing.T) {
	for in, want := range map[string]string{
		"SKY CINEMA PREMIEREN [SAT] []": "skycinemapremieren",
		"Sky Cinema Premieren HD":       "skycinemapremieren",
		"SKY SPORT F1 (BACKUP) HD+":     "skysportf1",
		"Das Erste UHD":                 "daserste",
		"HD":                            "hd",
	} {
		if got := NormName(in); got != want {
			t.Errorf("NormName(%q) = %q, want %q", in, got, want)
		}
	}
}
