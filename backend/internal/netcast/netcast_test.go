package netcast

import (
	"strings"
	"testing"
)

func TestParse_ServerFileHidesLogins(t *testing.T) {
	f, err := Parse("netcastserver_x.json", []byte(`{"serverlist":[
		{"id":"a","name":"A","mainUrl":"https://u:p@h.example/list.m3u?token=s3cret","mediatype":"playlist_live"},
		{"id":"b","mediatype":"api","Username":"me","Password":"hunter2"}], "password":"top"}`))
	if err != nil || f.Kind != "server" || len(f.Sources) != 2 {
		t.Fatalf("%+v %v", f, err)
	}
	if f.Sources[0].Address != "https://h.example/list.m3u" || !f.Sources[0].HasLogin {
		t.Errorf("a: %+v", f.Sources[0])
	}
	if f.Sources[1].Name != "b" || !f.Sources[1].HasLogin {
		t.Errorf("b: %+v", f.Sources[1])
	}
	if len(f.Settings) != 0 {
		t.Errorf("settings: %+v", f.Settings)
	}
}

func TestParse_EPGSettingsAndErrors(t *testing.T) {
	f, err := Parse("netcastepg_g.json", []byte(`{"EPGrefreshHour":"96","epglist":[{"name":"E","epgUrl":"https://e.example/x.xml.gz"}]}`))
	if err != nil || f.Settings["EPGrefreshHour"] != "96" || f.Sources[0].Address != "https://e.example/x.xml.gz" {
		t.Fatalf("%+v %v", f, err)
	}
	for name, body := range map[string]string{"other.json": "{}", "netcastserver_x.json": "nope"} {
		if _, err := Parse(name, []byte(body)); err == nil || strings.Contains(err.Error(), "nope") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
