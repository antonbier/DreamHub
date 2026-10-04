package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

type radioStation struct {
	ServiceReference string `json:"serviceReference"`
	ServiceName      string `json:"serviceName"`
	Now              *struct {
		Title string `json:"title"`
	} `json:"now"`
	Next *struct {
		Title string `json:"title"`
	} `json:"next"`
}

func TestRadio_BouquetsAndStations(t *testing.T) {
	m := mock.New()
	m.Now = func() time.Time { return mock.Evening.Add(21*time.Hour + 10*time.Minute) }
	e := newEnvWith(t, m)

	_, b := e.do(t, "GET", "/api/v1/radio/bouquets", "", nil)
	var bouquets struct {
		Bouquets []webcontrol.Service `json:"bouquets"`
	}
	json.Unmarshal(b, &bouquets)
	// The radio root's bouquet, then the list of every radio service
	// (unnamed — the screen names it).
	if len(bouquets.Bouquets) != 2 || bouquets.Bouquets[0].ServiceReference != mock.FavouritesRadio.ServiceReference ||
		bouquets.Bouquets[1].ServiceReference != webcontrol.RadioAll || bouquets.Bouquets[1].ServiceName != "" {
		t.Fatalf("expected the radio bouquet from the radio root and all stations, got %s", b)
	}

	// Play a station first, so the list can mark it.
	if resp, b := e.do(t, "POST", "/api/v1/control/zap", `{"serviceReference":"`+mock.RadioSWR3.ServiceReference+`"}`, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("zap: %d %s", resp.StatusCode, b)
	}

	resp, b := e.do(t, "GET", "/api/v1/radio/stations?"+url.Values{"bouquet": {mock.FavouritesRadio.ServiceReference}}.Encode(), "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Stations []radioStation `json:"stations"`
		Playing  string         `json:"playing"`
	}
	json.Unmarshal(b, &body)
	// Bouquet order, the heading dropped, the station without EPG kept.
	var names []string
	for _, s := range body.Stations {
		names = append(names, s.ServiceName)
	}
	if len(names) != 4 || names[0] != "Deutschlandfunk" || names[1] != "BAYERN 3" || names[3] != "Klassik Radio" {
		t.Fatalf("unexpected stations %v", names)
	}
	dlf := body.Stations[0]
	if dlf.Now == nil || dlf.Now.Title != "Hörspiel: Der Process" || dlf.Next == nil || dlf.Next.Title != "Musikszene" {
		t.Errorf("Deutschlandfunk now/next at 21:10: %+v", dlf)
	}
	if klassik := body.Stations[3]; klassik.Now != nil || klassik.Next != nil {
		t.Errorf("Klassik Radio has no EPG, got %+v", klassik)
	}
	if body.Playing != mock.RadioSWR3.ServiceReference {
		t.Errorf("expected SWR3 marked as playing, got %q", body.Playing)
	}

	if resp, _ := e.do(t, "GET", "/api/v1/radio/stations", "", nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing bouquet: expected 400, got %d", resp.StatusCode)
	}
}

func TestRadio_TimersAreRadioOnlyAndUpcoming(t *testing.T) {
	m := mock.New()
	ctx := context.Background()
	at := func(h, min int) int64 {
		return mock.Evening.Add(time.Duration(h)*time.Hour + time.Duration(min)*time.Minute).Unix()
	}
	// One radio timer still to come, one long past; the fixture TV timers
	// must not show up.
	m.AddTimer(ctx, webcontrol.NewTimer{ServiceReference: mock.RadioDLF.ServiceReference, Name: "Hörspiel: Der Process", Begin: at(21, 3), End: at(22, 2)})
	m.AddTimer(ctx, webcontrol.NewTimer{ServiceReference: mock.RadioSWR3.ServiceReference, Name: "Gestern", Begin: at(-20, 0), End: at(-19, 0)})
	e := newEnvWith(t, m)

	_, b := e.do(t, "GET", "/api/v1/radio/timers", "", nil)
	var body struct {
		Timers []webcontrol.Timer `json:"timers"`
	}
	json.Unmarshal(b, &body)
	if len(body.Timers) != 1 || body.Timers[0].Name != "Hörspiel: Der Process" {
		t.Fatalf("expected just the upcoming radio timer, got %s", b)
	}
}

func TestRadio_SearchFindsStations(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/search?q=swr", "", nil)
	var body struct {
		Results []struct {
			HubID string `json:"hubId"`
			Title string `json:"title"`
			Path  string `json:"path"`
		} `json:"results"`
	}
	json.Unmarshal(b, &body)
	var found bool
	for _, r := range body.Results {
		if r.HubID == "radio" && r.Title == "SWR3" {
			found = true
			u, err := url.Parse(r.Path)
			if err != nil || u.Path != "/radio" || u.Query().Get("station") != mock.RadioSWR3.ServiceReference {
				t.Errorf("unexpected path %q", r.Path)
			}
		}
	}
	if !found {
		t.Fatalf("expected SWR3 among the results, got %s", b)
	}
}

// Recording a radio programme is TimerHub's ordinary plan → apply; the new
// timer then shows up among RadioHub's radio recordings.
func TestRadio_RecordingAProgrammeShowsUpAsRadioTimer(t *testing.T) {
	e := newEnv(t)
	body := fmt.Sprintf(`{"serviceReference":%q,"serviceName":"Deutschlandfunk","name":"Hörspiel: Der Process","begin":%d,"end":%d,"eventId":103}`,
		mock.RadioDLF.ServiceReference, ev(21, 5), ev(22, 0))
	p := e.planTimer(t, body)
	if resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("apply: %d %s", resp.StatusCode, b)
	}
	_, b := e.do(t, "GET", "/api/v1/radio/timers", "", nil)
	var timers struct {
		Timers []webcontrol.Timer `json:"timers"`
	}
	json.Unmarshal(b, &timers)
	if len(timers.Timers) != 1 || timers.Timers[0].Name != "Hörspiel: Der Process" || timers.Timers[0].ServiceReference != mock.RadioDLF.ServiceReference {
		t.Fatalf("expected the new radio timer, got %s", b)
	}
}

// All radio stations: unnamed data services dropped, a station on two
// transponders once — the reference box's list looks like that.
func TestRadio_AllStationsAreCleanedUp(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/radio/stations?"+url.Values{"bouquet": {webcontrol.RadioAll}}.Encode(), "", nil)
	var body struct {
		Stations []radioStation `json:"stations"`
	}
	json.Unmarshal(b, &body)
	var names []string
	for _, s := range body.Stations {
		names = append(names, s.ServiceName)
	}
	if want := []string{"BAYERN 3", "Deutschlandfunk", "Klassik Radio", "SWR3"}; !slices.Equal(names, want) {
		t.Fatalf("stations %v, want %v", names, want)
	}
	if body.Stations[0].ServiceReference != mock.RadioBayern3.ServiceReference || body.Stations[0].Now == nil {
		t.Errorf("BAYERN 3 should be the one with programme information: %+v", body.Stations[0])
	}
}
