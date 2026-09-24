package hubs

import (
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

func TestGroupNowNext(t *testing.T) {
	events := []webcontrol.EPGEvent{
		{Title: "RTL Aktuell", ServiceReference: "rtl", ServiceName: "RTL Television"},
		{Title: "Bauer sucht Frau", ServiceReference: "rtl", ServiceName: "RTL Television"},
		{Title: "heute-show", ServiceReference: "zdfneo", ServiceName: "zdf_neo HD"},
		{Title: "The Rookie", ServiceReference: "zdfneo", ServiceName: "zdf_neo HD"},
	}
	got := groupNowNext(events)
	if len(got) != 2 {
		t.Fatalf("expected 2 channels, got %d: %+v", len(got), got)
	}
	if got[0].ServiceReference != "rtl" || got[0].Now.Title != "RTL Aktuell" || got[0].Next.Title != "Bauer sucht Frau" {
		t.Errorf("channel 0 (order must be preserved): %+v", got[0])
	}
	if got[1].ServiceReference != "zdfneo" || got[1].Now.Title != "heute-show" || got[1].Next.Title != "The Rookie" {
		t.Errorf("channel 1: %+v", got[1])
	}
}

func TestGroupNowNext_MoreThanTwoEventsIgnoresTheRest(t *testing.T) {
	events := []webcontrol.EPGEvent{
		{Title: "A", ServiceReference: "x"},
		{Title: "B", ServiceReference: "x"},
		{Title: "C", ServiceReference: "x"}, // a third event for the same channel — not now/next, dropped
	}
	got := groupNowNext(events)
	if len(got) != 1 || got[0].Now.Title != "A" || got[0].Next.Title != "B" {
		t.Errorf("unexpected: %+v", got)
	}
}
