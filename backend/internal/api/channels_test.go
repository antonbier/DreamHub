package api_test

import (
	"encoding/json"
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
	m.ScanChannel(webcontrol.Service{ServiceReference: "1:0:19:3001:441:1:C00000:0:0:0:", ServiceName: "Neu HD"})
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

type entriesResp struct {
	Entries []struct {
		Ref    string `json:"ref"`
		Name   string `json:"name"`
		Marker bool   `json:"marker"`
	} `json:"entries"`
}

const freeTV = `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`

func (e *env) entries(t *testing.T, ref string) entriesResp {
	t.Helper()
	var r entriesResp
	e.json(t, "GET", "/api/v1/channels/bouquet?ref="+urlq(ref), "", nil, 200, &r)
	return r
}

func (e *env) planAndApply(t *testing.T, path, body string, wantApply int) (planResp, []byte) {
	t.Helper()
	var p planResp
	e.json(t, "POST", path, body, nil, 200, &p)
	resp, b := e.do(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", nil)
	if resp.StatusCode != wantApply {
		t.Fatalf("apply: expected %d, got %d: %s", wantApply, resp.StatusCode, b)
	}
	return p, b
}

func TestChannels_EditBouquetAsOnePlan(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	// Free TV is RTL, zdf_neo. Wanted: a heading, ZDF HD (new), zdf_neo
	// first, RTL renamed; nothing removed yet.
	draft := `{"ref":` + jsonStr(freeTV) + `,"kind":"tv","entries":[
		{"name":"Öffentlich-Rechtliche","marker":true},
		{"ref":"` + mock.TVZDF.ServiceReference + `","name":"ZDF HD"},
		{"ref":"` + mock.TVZDFNeo.ServiceReference + `","name":"zdf_neo HD"},
		{"ref":"` + mock.TVRTL.ServiceReference + `","name":"RTL"}]}`
	var p struct {
		Plan struct {
			Changes []struct {
				Type string `json:"type"`
			} `json:"changes"`
		} `json:"plan"`
	}
	e.json(t, "POST", "/api/v1/channels/bouquet/plan", draft, nil, 200, &p)
	types := map[string]int{}
	for _, c := range p.Plan.Changes {
		types[c.Type]++
	}
	if types["marker.add"] != 1 || types["channel.add"] != 1 || types["channel.rename"] != 1 || types["channel.move"] != 1 || types["channel.remove"] != 0 {
		t.Fatalf("preview: %+v", types)
	}
	e.planAndApply(t, "/api/v1/channels/bouquet/plan", draft, 200)
	got := e.entries(t, freeTV)
	names := []string{}
	for _, x := range got.Entries {
		names = append(names, x.Name)
	}
	if strings.Join(names, "|") != "Öffentlich-Rechtliche|ZDF HD|zdf_neo HD|RTL" || !got.Entries[0].Marker {
		t.Fatalf("after apply: %v", names)
	}

	// Now remove RTL and the heading — references as the box made them.
	draft2 := `{"ref":` + jsonStr(freeTV) + `,"kind":"tv","entries":[
		{"ref":"` + mock.TVZDF.ServiceReference + `","name":"ZDF HD"},
		{"ref":"` + mock.TVZDFNeo.ServiceReference + `","name":"zdf_neo HD"}]}`
	e.planAndApply(t, "/api/v1/channels/bouquet/plan", draft2, 200)
	if n := len(e.entries(t, freeTV).Entries); n != 2 {
		t.Fatalf("after removing: %d entries", n)
	}
	// The same draft again changes nothing.
	if resp, b := e.do(t, "POST", "/api/v1/channels/bouquet/plan", draft2, nil); resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "channels_unchanged") {
		t.Fatalf("unchanged: %d %s", resp.StatusCode, b)
	}
	// Both changes left a backup each.
	var backups struct {
		Backups []struct {
			Filename string `json:"filename"`
		} `json:"backups"`
	}
	e.json(t, "GET", "/api/v1/channels/backups", "", nil, 200, &backups)
	if len(backups.Backups) == 0 {
		t.Fatal("no backup recorded")
	}
}

func TestChannels_FailedStepRestoresTheBackup(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	before := e.entries(t, freeTV)
	m.FailEditor = "moveservice"
	draft := `{"ref":` + jsonStr(freeTV) + `,"kind":"tv","entries":[
		{"ref":"` + mock.TVArte.ServiceReference + `","name":"arte HD"},
		{"ref":"` + mock.TVZDFNeo.ServiceReference + `","name":"zdf_neo HD"},
		{"ref":"` + mock.TVRTL.ServiceReference + `","name":"RTL Television"}]}`
	_, b := e.planAndApply(t, "/api/v1/channels/bouquet/plan", draft, http.StatusConflict)
	if !strings.Contains(string(b), "channels_restored") || m.Restores != 1 {
		t.Fatalf("expected a restore: %s (restores %d)", b, m.Restores)
	}
	after := e.entries(t, freeTV)
	if len(after.Entries) != len(before.Entries) || after.Entries[0].Ref != before.Entries[0].Ref {
		t.Fatalf("the bouquet wasn't restored: %+v", after)
	}
}

func TestChannels_EditingNeedsTheBouquetEditor(t *testing.T) {
	m := mock.New()
	m.NoBouquetEditor = true
	e := newEnvWith(t, m)
	resp, b := e.do(t, "POST", "/api/v1/channels/bouquet/plan", `{"ref":`+jsonStr(freeTV)+`,"kind":"tv","entries":[]}`, nil)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), "channels_no_editor") {
		t.Fatalf("no editor: %d %s", resp.StatusCode, b)
	}
}

func TestChannels_DraftValidation(t *testing.T) {
	e := newEnv(t)
	dup := `{"ref":` + jsonStr(freeTV) + `,"kind":"tv","entries":[
		{"ref":"` + mock.TVRTL.ServiceReference + `","name":"RTL"},{"ref":"` + mock.TVRTL.ServiceReference + `","name":"RTL"}]}`
	if resp, b := e.do(t, "POST", "/api/v1/channels/bouquet/plan", dup, nil); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "duplicate") {
		t.Fatalf("duplicate: %d %s", resp.StatusCode, b)
	}
}

func TestChannels_BouquetsAndRestore(t *testing.T) {
	m := mock.New()
	e := newEnvWith(t, m)
	e.planAndApply(t, "/api/v1/channels/bouquets/plan", `{"action":"create","kind":"tv","name":"Kinder"}`, 200)
	var list struct {
		Bouquets []struct {
			Ref  string `json:"ref"`
			Name string `json:"name"`
		} `json:"bouquets"`
	}
	e.json(t, "GET", "/api/v1/channels/bouquets", "", nil, 200, &list)
	var kids string
	for _, b := range list.Bouquets {
		if b.Name == "Kinder" {
			kids = b.Ref
		}
	}
	if kids == "" {
		t.Fatalf("created bouquet missing: %+v", list)
	}
	e.planAndApply(t, "/api/v1/channels/bouquets/plan", `{"action":"rename","kind":"tv","ref":`+jsonStr(kids)+`,"name":"Kids"}`, 200)
	e.planAndApply(t, "/api/v1/channels/bouquets/plan", `{"action":"delete","kind":"tv","ref":`+jsonStr(kids)+`}`, 200)
	e.json(t, "GET", "/api/v1/channels/bouquets", "", nil, 200, &list)
	if len(list.Bouquets) != 2 {
		t.Fatalf("after delete: %+v", list)
	}

	// The backup made before "create" still has no "Kinder"; the one before
	// "delete" has "Kids". Restoring that brings it back.
	var backups struct {
		Backups []struct {
			Filename string `json:"filename"`
			Summary  string `json:"summary"`
		} `json:"backups"`
	}
	e.json(t, "GET", "/api/v1/channels/backups", "", nil, 200, &backups)
	var beforeDelete string
	for _, b := range backups.Backups {
		if strings.HasPrefix(b.Summary, "Deleted") {
			beforeDelete = b.Filename
		}
	}
	if beforeDelete == "" {
		t.Fatalf("backups: %+v", backups)
	}
	e.planAndApply(t, "/api/v1/channels/backups/restore/plan", `{"filename":`+jsonStr(beforeDelete)+`}`, 200)
	e.json(t, "GET", "/api/v1/channels/bouquets", "", nil, 200, &list)
	if !strings.Contains(fmtBouquets(list.Bouquets), "Kids") {
		t.Fatalf("restore didn't bring Kids back: %+v", list)
	}
}

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

func fmtBouquets(l []struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
}) string {
	var names []string
	for _, b := range l {
		names = append(names, b.Name)
	}
	return strings.Join(names, ",")
}
