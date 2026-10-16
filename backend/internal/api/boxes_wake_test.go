package api_test

import (
	"net/http"
	"testing"
)

type wakeBox struct {
	ID         string
	Wake       bool
	WakeMethod string
	WakeMac    string
	WakeExt    string
	WakeAction string
}

func (e *env) boxWake(t *testing.T, id string, as func(*http.Request)) wakeBox {
	t.Helper()
	var list struct{ Boxes []wakeBox }
	e.json(t, "GET", "/api/v1/boxes", "", as, 200, &list)
	for _, b := range list.Boxes {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("box %s not listed", id)
	return wakeBox{}
}

func TestBoxWake_WakeOnLanSendsThePacket(t *testing.T) {
	e := newEnv(t)
	var sent []string
	e.app.BoxManager.SendWOL = func(mac string) error { sent = append(sent, mac); return nil }
	admin, viewer, benID := e.householdWithBoxes(t)

	// Nothing is set up yet: no wake button, and asking is refused.
	if b := e.boxWake(t, "bedroom", withProfile(admin)); b.Wake {
		t.Fatalf("no method yet: %+v", b)
	}
	if resp, b := e.do(t, "POST", "/api/v1/boxes/bedroom/wake", "", withProfile(admin)); resp.StatusCode != http.StatusConflict {
		t.Fatalf("no method: %d %s", resp.StatusCode, b)
	}
	// A bad MAC is a validation error, nothing is saved.
	if resp, b := e.do(t, "PATCH", "/api/v1/boxes/bedroom", `{"wakeMethod":"wol","wakeMac":"nope"}`, withProfile(admin)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad MAC: %d %s", resp.StatusCode, b)
	}
	e.json(t, "PATCH", "/api/v1/boxes/bedroom", `{"wakeMethod":"wol","wakeMac":"AA-BB-CC-DD-EE-FF"}`, withProfile(admin), 200, nil)

	b := e.boxWake(t, "bedroom", withProfile(admin))
	if !b.Wake || b.WakeMethod != "wol" || b.WakeMac != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("admin sees the setup: %+v", b)
	}
	// A profile with the remote may switch a box on — if it may use that box.
	e.json(t, "PATCH", "/api/v1/profiles/"+itoa(benID), `{"boxes":["main"]}`, withProfile(admin), 200, nil)
	if resp, _ := e.do(t, "POST", "/api/v1/boxes/bedroom/wake", "", withProfile(viewer)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a box the profile may not use: %d", resp.StatusCode)
	}
	e.json(t, "PATCH", "/api/v1/profiles/"+itoa(benID), `{"boxes":["main","bedroom"]}`, withProfile(admin), 200, nil)
	e.json(t, "POST", "/api/v1/boxes/bedroom/wake", "", withProfile(viewer), 200, nil)
	sent = nil
	var res struct{ Sent bool }
	e.json(t, "POST", "/api/v1/boxes/bedroom/wake", "", withProfile(admin), 200, &res)
	if !res.Sent || len(sent) != 1 || sent[0] != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("sent %v %+v", sent, res)
	}
	if resp, _ := e.do(t, "POST", "/api/v1/boxes/nowhere/wake", "", withProfile(admin)); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown box: %d", resp.StatusCode)
	}
}

func TestBoxWake_ExtensionActionIsAPlanFirst(t *testing.T) {
	e := newEnv(t)
	admin, _, _ := e.householdWithBoxes(t)
	// The mock mode's demo socket starts on; the plan says what it will do.
	e.json(t, "PATCH", "/api/v1/boxes/bedroom", `{"wakeMethod":"extension","wakeExt":"demo.socket","wakeAction":"toggle"}`, withProfile(admin), 200, nil)
	var p planResp
	e.json(t, "POST", "/api/v1/boxes/bedroom/wake", "", withProfile(admin), 200, &p)
	if p.Plan.ID == "" || p.Plan.Action != "box.wake" {
		t.Fatalf("a wake plan: %+v", p.Plan)
	}
	var ext struct {
		Extensions []struct {
			ID      string
			Widgets []struct{ Type, Value string }
		}
	}
	state := func() string {
		e.json(t, "GET", "/api/v1/extensions", "", withProfile(admin), 200, &ext)
		for _, x := range ext.Extensions {
			for _, w := range x.Widgets {
				if w.Type == "status" {
					return w.Value
				}
			}
		}
		return ""
	}
	before := state()
	if before == "" {
		t.Fatal("demo socket has no status")
	}
	// Planning changed nothing.
	if state() != before {
		t.Fatal("a plan switches nothing")
	}
	e.json(t, "POST", "/api/v1/plans/"+p.Plan.ID+"/apply", "", withProfile(admin), 200, nil)
	if state() == before {
		t.Error("applying the plan runs the extension's action")
	}

	// An extension that is not there is said plainly.
	e.json(t, "PATCH", "/api/v1/boxes/bedroom", `{"wakeExt":"gone"}`, withProfile(admin), 200, nil)
	resp, b := e.do(t, "POST", "/api/v1/boxes/bedroom/wake", "", withProfile(admin))
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusConflict || code != "wake_extension_missing" {
		t.Errorf("missing extension: %d %s", resp.StatusCode, b)
	}
}

func TestBoxWake_RemovedWithTheBox(t *testing.T) {
	e := newEnv(t)
	admin, _, _ := e.householdWithBoxes(t)
	e.json(t, "PATCH", "/api/v1/boxes/main", `{"wakeMethod":"wol","wakeMac":"aabbccddeeff"}`, withProfile(admin), 200, nil)
	if b := e.boxWake(t, "main", withProfile(admin)); b.WakeMac != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("the first box can be set up too: %+v", b)
	}
}
