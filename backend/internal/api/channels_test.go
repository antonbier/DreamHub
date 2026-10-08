package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

type channelList struct {
	Channels []struct {
		Ref      string   `json:"ref"`
		Name     string   `json:"name"`
		Provider string   `json:"provider"`
		Kind     string   `json:"kind"`
		Quality  string   `json:"quality"`
		New      bool     `json:"new"`
		Bouquets []string `json:"bouquets"`
	} `json:"channels"`
}

func TestChannels_BouquetsAndEntries(t *testing.T) {
	e := newEnv(t)
	var list struct {
		Bouquets []struct {
			Ref   string `json:"ref"`
			Name  string `json:"name"`
			Kind  string `json:"kind"`
			Count int    `json:"count"`
		} `json:"bouquets"`
		Editor bool `json:"editor"`
	}
	e.json(t, "GET", "/api/v1/channels/bouquets", "", nil, 200, &list)
	if len(list.Bouquets) != 2 || list.Bouquets[0].Kind != "tv" || list.Bouquets[1].Kind != "radio" || !list.Editor {
		t.Fatalf("bouquets: %+v", list)
	}
	if list.Bouquets[1].Count != 4 {
		t.Fatalf("radio favourites: 4 stations and a heading, counted %d", list.Bouquets[1].Count)
	}
	var radio struct {
		Entries []struct {
			Name   string `json:"name"`
			Marker bool   `json:"marker"`
		} `json:"entries"`
	}
	e.json(t, "GET", "/api/v1/channels/bouquet?ref="+urlq(list.Bouquets[1].Ref), "", nil, 200, &radio)
	if len(radio.Entries) != 5 || !radio.Entries[1].Marker {
		t.Fatalf("radio entries with their heading: %+v", radio)
	}
}

func TestChannels_FindsNewAndUnusedChannels(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	var first channelList
	e.json(t, "GET", "/api/v1/channels/all", "", nil, 200, &first)
	byName := map[string]int{}
	for i, c := range first.Channels {
		byName[c.Name] = i
		if c.New {
			t.Fatalf("on the first look nothing is new: %+v", c)
		}
	}
	arte := first.Channels[byName["arte HD"]]
	if arte.Provider != "ARD" || arte.Quality != "hd" || len(arte.Bouquets) != 0 {
		t.Fatalf("arte: %+v", arte)
	}
	rtl := first.Channels[byName["RTL Television"]]
	if len(rtl.Bouquets) != 1 || rtl.Bouquets[0] != "Free TV" {
		t.Fatalf("RTL is in the favourites: %+v", rtl)
	}
	if k1 := first.Channels[byName["kabel eins"]]; k1.Quality != "sd" {
		t.Fatalf("kabel eins is SD: %+v", k1)
	}

	// A channel scan brings a new one; the next look marks it.
	m.AddChannel(webcontrol.Service{ServiceReference: "1:0:19:3001:441:1:C00000:0:0:0:", ServiceName: "Neu HD"})
	var second channelList
	e.json(t, "GET", "/api/v1/channels/all?refresh=1", "", nil, 200, &second)
	newOnes := 0
	for _, c := range second.Channels {
		if c.New {
			newOnes++
			if c.Name != "Neu HD" || c.Provider != "New" {
				t.Fatalf("new channel: %+v", c)
			}
		}
	}
	if newOnes != 1 {
		t.Fatalf("expected one new channel, got %d", newOnes)
	}
}

func TestChannels_CheckAndPicons(t *testing.T) {
	e := newEnv(t)
	var check struct {
		Issues []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"issues"`
	}
	e.json(t, "GET", "/api/v1/channels/check", "", nil, 200, &check)
	kinds := map[string]bool{}
	for _, i := range check.Issues {
		kinds[i.Kind] = true
	}
	// Off the box there are no picons at all.
	if !kinds["noPicon"] {
		t.Fatalf("check: %+v", check)
	}
	var picons struct {
		Total int `json:"total"`
	}
	e.json(t, "GET", "/api/v1/channels/picons", "", nil, 200, &picons)
	if picons.Total == 0 {
		t.Fatal("picons: no channels counted")
	}
	if resp, _ := e.do(t, "PUT", "/api/v1/channels/picons", `{"set":"nope"}`, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown set: expected 404, got %d", resp.StatusCode)
	}
	e.json(t, "PUT", "/api/v1/channels/picons", `{"set":""}`, nil, 200, nil)
}

func TestChannels_AdminOnly(t *testing.T) {
	e := newEnv(t)
	_, viewer, _ := e.household(t)
	if resp, _ := e.do(t, "GET", "/api/v1/channels/bouquets", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer: expected 403, got %d", resp.StatusCode)
	}
	_, b := e.do(t, "GET", "/api/v1/navigation", "", withProfile(viewer))
	if strings.Contains(string(b), `"/channels"`) {
		t.Fatal("viewer navigation shows ChannelHub")
	}
}

func urlq(s string) string {
	return strings.NewReplacer("%", "%25", " ", "%20", `"`, "%22", "&", "%26", "+", "%2B", "#", "%23").Replace(s)
}
