package hubs

import (
	"context"
	"database/sql"
	"github.com/antonbier/DreamHub/backend/internal/titles"
	"github.com/antonbier/DreamHub/backend/internal/tvmaze"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/dmi"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

func TestEpisodeTitleCandidates(t *testing.T) {
	got := episodeTitleCandidates("FBI: Most Wanted - Gefangenenübergabe", "Der Gefangene. (USA 2024)\nKrimiserie", "FBI: Most Wanted")
	want := []string{"Der Gefangene. (USA 2024)", "Gefangenenübergabe", "Der Gefangene"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if got := episodeTitleCandidates("Tatort", "Tatort", "Tatort"); len(got) != 0 {
		t.Errorf("the series name itself is no episode title: %q", got)
	}
}

func TestAnnotateBroadcasts(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dmi.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE media_cache (id INTEGER PRIMARY KEY AUTOINCREMENT, key TEXT UNIQUE, title TEXT, type TEXT, data TEXT, ts INTEGER)`,
		`INSERT INTO media_cache (key, title, type, data, ts) VALUES ('the rookie_tv', 'The Rookie', 'tv', '{"id":79744,"name":"The Rookie"}', 1)`,
		`CREATE TABLE match_cache (key TEXT PRIMARY KEY, se TEXT)`,
		`INSERT INTO match_cache (key, se) VALUES ('79744|Verdeckt', 'S02E05'), ('79744|Zurück', 'S02E01'), ('79744|Neu', 'S07E01')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	h := &Series{DMI: &dmi.Reader{Path: p}}
	m := serienrecorder.Marker{Name: "The Rookie"}
	bs := []seriesBroadcast{
		{EPGEvent: webcontrol.EPGEvent{Title: "The Rookie", Description: "Verdeckt\nKrimiserie"}},
		{EPGEvent: webcontrol.EPGEvent{Title: "The Rookie", Description: "Zurück"}},
		{EPGEvent: webcontrol.EPGEvent{Title: "The Rookie - S03E02"}},
		{EPGEvent: webcontrol.EPGEvent{Title: "The Rookie", Description: "Neu"}},
		{EPGEvent: webcontrol.EPGEvent{Title: "The Rookie", Description: "Unbekannt"}},
	}
	recs := []seriesRecording{{Season: 2, Episode: 1}}
	upcoming := []serienrecorder.UpcomingEpisode{{Season: "7", Episode: "1"}}
	h.annotateBroadcasts(context.Background(), m, bs, recs, upcoming, nil)

	type got struct {
		S, E           int
		Source, Status string
	}
	want := []got{
		{2, 5, "dreammediainfo", BroadcastMissing},
		{2, 1, "dreammediainfo", BroadcastRecorded},
		{3, 2, "epg", BroadcastMissing},
		{7, 1, "dreammediainfo", BroadcastPlanned},
		{0, 0, "", ""},
	}
	for i, b := range bs {
		if g := (got{b.Season, b.Episode, b.CodeSource, b.EpisodeStatus}); g != want[i] {
			t.Errorf("broadcast %d: %+v, want %+v", i, g, want[i])
		}
	}

	// Absolute numbering (S00E8615): nothing can be claimed missing.
	bs = []seriesBroadcast{{EPGEvent: webcontrol.EPGEvent{Title: "The Rookie", Description: "Verdeckt"}}}
	h.annotateBroadcasts(context.Background(), m, bs, []seriesRecording{{Season: 0, Episode: 8615}}, nil, nil)
	if bs[0].EpisodeStatus != BroadcastUnmatched {
		t.Errorf("without season codes, no claim: %q", bs[0].EpisodeStatus)
	}
}

func TestAnnotateBroadcasts_Absolute(t *testing.T) {
	m := serienrecorder.Marker{Name: "Gute Zeiten, schlechte Zeiten"}
	guide := &episodeGuide{Absolute: true, absFrom: 8613, absNames: map[string]int{
		titles.Normalize("Hirngespinst"): 8614, titles.Normalize("Folge 1"): 1,
	}}
	recs := []seriesRecording{{Season: 0, Episode: 8613, Description: "S00E8613 - (K)Ein Abschied"}}
	upcoming := []serienrecorder.UpcomingEpisode{{Season: "0", Episode: "8615", Title: "Fehler der Vergangenheit"}}
	bs := []seriesBroadcast{
		{EPGEvent: webcontrol.EPGEvent{Title: "Gute Zeiten, schlechte Zeiten", Description: "(K)Ein Abschied"}},
		{EPGEvent: webcontrol.EPGEvent{Title: "Gute Zeiten, schlechte Zeiten", Description: "Hirngespinst\nDaily Soap"}},
		{EPGEvent: webcontrol.EPGEvent{Title: "Gute Zeiten, schlechte Zeiten", Description: "Fehler der Vergangenheit"}},
		{EPGEvent: webcontrol.EPGEvent{Title: "Gute Zeiten, schlechte Zeiten", Description: "Folge 1"}},
		{EPGEvent: webcontrol.EPGEvent{Title: "Gute Zeiten, schlechte Zeiten", Description: "Unbekannt"}},
	}
	(&Series{}).annotateBroadcasts(context.Background(), m, bs, recs, upcoming, guide)
	type got struct {
		E              int
		Source, Status string
	}
	want := []got{
		{8613, "recording", BroadcastRecorded},
		{8614, "tvmaze", BroadcastMissing},
		{8615, "serienrecorder", BroadcastPlanned},
		{1, "tvmaze", BroadcastUnmatched}, // before the first recording
		{0, "", ""},
	}
	for i, b := range bs {
		if g := (got{b.Episode, b.CodeSource, b.EpisodeStatus}); g != want[i] || b.Season != 0 {
			t.Errorf("broadcast %d: %+v (season %d), want %+v", i, g, b.Season, want[i])
		}
	}
}

func TestAbsoluteNumbering(t *testing.T) {
	eps := []tvmaze.Episode{{Season: 2026, Number: 8613, Name: "(K)Ein Abschied"}, {Season: 2026, Number: 8614, Name: "Hirngespinst"}}
	recs := []seriesRecording{
		{Episode: 8614, Description: "S00E8614 - Hirngespinst"},
		{Episode: 8620, Title: "Gute Zeiten, schlechte Zeiten - S00E8620 - Antipathie"},
	}
	if from, ok := absoluteNumbering(eps, recs); !ok || from != 8614 {
		t.Errorf("number and title agree: %d %v", from, ok)
	}
	if _, ok := absoluteNumbering(eps, []seriesRecording{{Episode: 8614, Description: "S00E8614 - Ganz anders"}}); ok {
		t.Error("a number with another title must not count")
	}
	dup := append(eps, tvmaze.Episode{Season: 2027, Number: 8614, Name: "Hirngespinst"})
	if _, ok := absoluteNumbering(dup, recs); ok {
		t.Error("numbers repeating across seasons aren't absolute")
	}
}

func TestMergeSimulcasts(t *testing.T) {
	ev := func(sref, name string, start int64, desc string) seriesBroadcast {
		return seriesBroadcast{EPGEvent: webcontrol.EPGEvent{ServiceReference: sref, ServiceName: name, Start: start, Duration: 2100, Title: "GZSZ", Description: desc}}
	}
	bs := []seriesBroadcast{
		ev("bayern", "RTL Bayern", 100, "Antragseifer"),
		ev("rtl", "RTL Television", 100, "Antragseifer"),
		ev("nrw", "RTL Regional NRW", 100, "Antragseifer"),
		ev("bayern", "RTL Bayern", 200, "Ja Doch"),
		ev("nrw", "RTL Regional NRW", 200, "Ja Doch"),
		ev("rtl", "RTL Television", 300, "Ja Doch"), // a repeat later: its own broadcast
	}
	bs[4].HasTimer = true
	out := mergeSimulcasts(bs, map[string]bool{"rtl": true})
	if len(out) != 3 {
		t.Fatalf("three broadcasts: %+v", out)
	}
	if out[0].ServiceName != "RTL Television" || strings.Join(out[0].AlsoOn, ",") != "RTL Bayern,RTL Regional NRW" {
		t.Errorf("the usual channel is kept: %s %v", out[0].ServiceName, out[0].AlsoOn)
	}
	if out[1].ServiceName != "RTL Regional NRW" || !out[1].HasTimer || strings.Join(out[1].AlsoOn, ",") != "RTL Bayern" {
		t.Errorf("the one with a timer is kept: %s %v", out[1].ServiceName, out[1].AlsoOn)
	}
	if out[2].Start != 300 || len(out[2].AlsoOn) != 0 {
		t.Errorf("the repeat stays: %+v", out[2])
	}
}
