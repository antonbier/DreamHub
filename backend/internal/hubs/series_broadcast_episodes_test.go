package hubs

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
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
	h.annotateBroadcasts(context.Background(), m, bs, recs, upcoming)

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
	h.annotateBroadcasts(context.Background(), m, bs, []seriesRecording{{Season: 0, Episode: 8615}}, nil)
	if bs[0].EpisodeStatus != BroadcastUnmatched {
		t.Errorf("without season codes, no claim: %q", bs[0].EpisodeStatus)
	}
}
