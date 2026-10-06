package hubs

import (
	"context"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

func TestGroupByChannel(t *testing.T) {
	events := []webcontrol.EPGEvent{
		{Title: "RTL Aktuell", ServiceReference: "rtl", ServiceName: "RTL Television"},
		{Title: "Bauer sucht Frau", ServiceReference: "rtl", ServiceName: "RTL Television"},
		{Title: "heute-show", ServiceReference: "zdfneo", ServiceName: "zdf_neo HD"},
		{Title: "The Rookie", ServiceReference: "zdfneo", ServiceName: "zdf_neo HD"},
		{Title: "Undercover Boss", ServiceReference: "rtl", ServiceName: "RTL Television"},
	}
	got := (&Guide{}).groupByChannel(context.Background(), events)
	if len(got) != 2 {
		t.Fatalf("expected 2 channels, got %d: %+v", len(got), got)
	}
	if got[0].ServiceReference != "rtl" || len(got[0].Events) != 3 {
		t.Errorf("channel 0 (order must be preserved, all 3 RTL events kept): %+v", got[0])
	}
	if got[0].Events[0].Title != "RTL Aktuell" || got[0].Events[2].Title != "Undercover Boss" {
		t.Errorf("channel 0 events must keep their given order: %+v", got[0].Events)
	}
	if got[1].ServiceReference != "zdfneo" || len(got[1].Events) != 2 {
		t.Errorf("channel 1: %+v", got[1])
	}
}

func TestTimerSource(t *testing.T) {
	for tags, want := range map[string]string{"SerienRecorder": SourceSerienRecorder, "AutoTimer Krimi": SourceAutoTimer, "": "", "Krimi": ""} {
		if got := timerSource(tags); got != want {
			t.Errorf("timerSource(%q) = %q, want %q", tags, got, want)
		}
	}
}
