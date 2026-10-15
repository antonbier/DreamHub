package appcatalog_test

import (
	"reflect"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
)

// The migration to compose files (App12) must change nothing about how a
// curated app installs: every resolved entry equals the Go struct it
// replaced (legacy_test.go) in every field the install flow reads.

func ports(in []legacyPort) []appcatalog.Port {
	var out []appcatalog.Port
	for _, p := range in {
		out = append(out, appcatalog.Port{Port: p.Port, UDP: p.UDP, Local: p.Local})
	}
	return out
}

func sameEnv(a, b map[string]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func sameArches(a, b []dockerhub.Arch) bool { return reflect.DeepEqual(a, b) }

func compare(t *testing.T, label string, want legacyApp, got appcatalog.App) {
	t.Helper()
	fail := func(field string, w, g any) { t.Errorf("%s: %s = %#v, want %#v", label, field, g, w) }
	if got.ID != want.ID {
		fail("ID", want.ID, got.ID)
	}
	if got.Name != want.Name {
		fail("Name", want.Name, got.Name)
	}
	if got.Category != want.Category {
		fail("Category", want.Category, got.Category)
	}
	if got.Description != want.Description {
		fail("Description", want.Description, got.Description)
	}
	if got.Note != want.Note {
		fail("Note", want.Note, got.Note)
	}
	if got.Image != want.Image || got.Tag != want.Tag {
		fail("Image:Tag", want.Image+":"+want.Tag, got.Image+":"+got.Tag)
	}
	if !sameArches(got.Arches, want.Arches) {
		fail("Arches", want.Arches, got.Arches)
	}
	if got.ContainerPort != want.ContainerPort {
		fail("ContainerPort", want.ContainerPort, got.ContainerPort)
	}
	if got.ConfigPath != want.ConfigPath || got.DataPath != want.DataPath {
		fail("ConfigPath/DataPath", want.ConfigPath+"|"+want.DataPath, got.ConfigPath+"|"+got.DataPath)
	}
	if got.LargeConfig != want.LargeConfig {
		fail("LargeConfig", want.LargeConfig, got.LargeConfig)
	}
	if !sameEnv(got.Env, want.Env) {
		fail("Env", want.Env, got.Env)
	}
	if !reflect.DeepEqual(got.Cmd, want.Cmd) && !(len(got.Cmd) == 0 && len(want.Cmd) == 0) {
		fail("Cmd", want.Cmd, got.Cmd)
	}
	if got.PortEditable() != want.portEditable() {
		fail("PortEditable", want.portEditable(), got.PortEditable())
	}
	if !reflect.DeepEqual(got.Ports, ports(want.Ports)) && !(len(got.Ports) == 0 && len(want.Ports) == 0) {
		fail("Ports", want.Ports, got.Ports)
	}
	if got.MemoryMB != want.MemoryMB {
		fail("MemoryMB", want.MemoryMB, got.MemoryMB)
	}
	if len(got.Services) != len(want.Services) {
		fail("len(Services)", len(want.Services), len(got.Services))
		return
	}
	for i, ws := range want.Services {
		gs := got.Services[i]
		sl := label + " service " + ws.Name
		if gs.Name != ws.Name || gs.Image != ws.Image || gs.Tag != ws.Tag || gs.ConfigPath != ws.ConfigPath || gs.MemoryMB != ws.MemoryMB {
			t.Errorf("%s: got %+v, want %+v", sl, gs, ws)
		}
		if !sameArches(gs.Arches, ws.Arches) || !sameEnv(gs.Env, ws.Env) || !reflect.DeepEqual(gs.Cmd, ws.Cmd) || !reflect.DeepEqual(gs.Ports, ports(ws.Ports)) {
			t.Errorf("%s: arches/env/cmd/ports differ: got %+v, want %+v", sl, gs, ws)
		}
	}
	if got.TotalMemoryMB() == 0 {
		t.Errorf("%s: no memory", label)
	}
}

func TestMigration_EveryAppEqualsItsFormerStruct(t *testing.T) {
	if len(appcatalog.Apps) != len(legacyApps) {
		t.Fatalf("%d apps, was %d", len(appcatalog.Apps), len(legacyApps))
	}
	for i, want := range legacyApps {
		compare(t, want.ID, want, appcatalog.Apps[i]) // same order, too
	}
}

// Moving the web port gives what the former WithPort gave.
func TestMigration_WithPortEqualsFormerWithPort(t *testing.T) {
	for _, want := range legacyApps {
		got, ok := appcatalog.ByID(want.ID)
		if !ok {
			t.Fatalf("%s missing", want.ID)
		}
		for _, port := range []int{9000, 9123, 8088, 8222} {
			compare(t, want.ID+" at "+string(rune('0'+port%10)), want.withPort(port), got.WithPort(port))
		}
	}
}

func TestMigration_FilesAndOrderAgree(t *testing.T) {
	var ids []string
	for _, a := range legacyApps {
		ids = append(ids, a.ID)
	}
	files := appcatalog.FileIDs()
	if len(files) != len(ids) {
		t.Fatalf("files %v, apps %v", files, ids)
	}
	for _, id := range ids {
		found := false
		for _, f := range files {
			found = found || f == id
		}
		if !found {
			t.Errorf("no file for %s", id)
		}
	}
}
