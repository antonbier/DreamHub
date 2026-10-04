package hubs

import (
	"context"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	srmock "github.com/antonbier/DreamHub/backend/internal/serienrecorder/mock"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

type fakeSR struct {
	serienrecorder.Client
	markers []serienrecorder.Marker
}

func (f fakeSR) Markers(context.Context) ([]serienrecorder.Marker, error) { return f.markers, nil }
func (f fakeSR) UpcomingEpisodes(context.Context) ([]serienrecorder.UpcomingEpisode, error) {
	return nil, nil
}

type epgBox struct {
	*mock.Client
	events []webcontrol.EPGEvent
}

func (b epgBox) EPGSearch(context.Context, string) ([]webcontrol.EPGEvent, error) {
	return b.events, nil
}

type recordedBus struct{ keys []string }

func (b *recordedBus) Publish(_ context.Context, key string, _ any) (store.Notification, error) {
	b.keys = append(b.keys, key)
	return store.Notification{}, nil
}

func TestCheckMissing(t *testing.T) {
	now := time.Date(2026, 10, 4, 18, 0, 0, 0, time.UTC)
	box := mock.New()
	box.FS.Put(mock.Root+"serien/FBI/Season 5/a.ts", "rec")
	box.FS.Put(mock.Root+"serien/FBI/Season 5/a.ts.meta", "1:0:1:1:1:1:C00000:0:0:0:\nFBI: Most Wanted - S05E07 - Gefangenenübergabe\nS05E07 - Gefangenenübergabe\n1745960400\n")
	ev := func(title string, in time.Duration) webcontrol.EPGEvent {
		return webcontrol.EPGEvent{Title: title, ServiceReference: "1:0:19:1:1:1:C00000:0:0:0:", ServiceName: "SAT.1", Start: now.Add(in).Unix(), Duration: 3600}
	}
	h := &Series{
		SR: fakeSR{Client: srmock.New(), markers: []serienrecorder.Marker{
			{ID: 1, Name: "FBI: Most Wanted", Active: true, RecordFolder: mock.Root + "serien/FBI"},
			{ID: 2, Name: "Paused", Active: false},
		}},
		WC: epgBox{Client: box, events: []webcontrol.EPGEvent{
			ev("FBI: Most Wanted - S05E08 - Neu", 3*time.Hour),                // missing, tonight: announced
			ev("FBI: Most Wanted - S05E07 - Gefangenenübergabe", 4*time.Hour), // recorded already
			ev("FBI: Most Wanted - S05E09 - Später", 40*time.Hour),            // missing, but not within a day
		}},
		FS:  box.FS,
		Now: func() time.Time { return now },
	}
	bus := &recordedBus{}
	seen := map[string]bool{}
	n, err := h.checkMissing(context.Background(), bus, seen)
	if err != nil || n != 1 || len(bus.keys) != 1 || bus.keys[0] != EventMissingAiring {
		t.Fatalf("one missing episode airing within a day: n=%d keys=%v err=%v", n, bus.keys, err)
	}
	if n, _ := h.checkMissing(context.Background(), bus, seen); n != 0 {
		t.Errorf("announced once per broadcast, got %d more", n)
	}
}
