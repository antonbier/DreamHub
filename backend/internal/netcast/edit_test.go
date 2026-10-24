package netcast

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const sample = `{"serverlist":[{"id":"a","name":"A","mainUrl":"https://h.example/a.m3u","mediatype":"playlist","Username":"me","Password":"hunter2","UpdateTime":6}],"Other":"keep"}`

func TestAddUpdateRemoveKeepUnknownFields(t *testing.T) {
	out, id, err := Add("server", []byte(sample), Input{Name: "My List", Address: "https://x.example/l.m3u", Desc: "d"})
	if err != nil || id != "dhmylist" {
		t.Fatalf("add: %v %q", err, id)
	}
	out, err = Update("server", out, "a", Input{Name: "A2"})
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	list := top["serverlist"].([]any)
	a := list[0].(map[string]any)
	if len(list) != 2 || a["name"] != "A2" || a["Password"] != "hunter2" || a["UpdateTime"] != float64(6) || top["Other"] != "keep" {
		t.Errorf("after edits: %s", out)
	}
	if b := list[1].(map[string]any); b["mediatype"] != "playlist_live" || b["mainUrl"] != "https://x.example/l.m3u" {
		t.Errorf("new entry: %v", b)
	}
	out, err = Remove("server", out, "a")
	if err != nil || strings.Contains(string(out), "hunter2") || !strings.Contains(string(out), "dhmylist") {
		t.Errorf("remove: %v %s", err, out)
	}
}

func TestNewFilesAndErrors(t *testing.T) {
	out, id, err := Add("epg", nil, Input{Name: "Guide", Address: "https://e.example/g.xml.gz"})
	if err != nil || id != "dhguide" || FileName("epg", id) != "netcastepg_dhguide.json" {
		t.Fatalf("%v %q", err, id)
	}
	f, err := Parse("netcastepg_dhguide.json", out)
	if err != nil || len(f.Sources) != 1 || f.Sources[0].Address != "https://e.example/g.xml.gz" {
		t.Errorf("round trip: %+v %v", f, err)
	}
	if _, _, err := Add("epg", out, Input{Name: "Guide", Address: "https://e.example/g2.xml"}); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate: %v", err)
	}
	for _, in := range []Input{{Name: "x"}, {Name: "x", Address: "ftp://h/x"}, {Name: "x", Address: "https://h/a b"}, {Name: "x", Address: "https://h/x", MediaType: "bogus"}} {
		if _, _, err := Add("server", nil, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", in, err)
		}
	}
	if _, err := Update("server", []byte(sample), "zzz", Input{Name: "n"}); !errors.Is(err, ErrNoSource) {
		t.Errorf("no source: %v", err)
	}
	if _, _, err := Add("server", []byte("[1,2]"), Input{Name: "x", Address: "https://h/x"}); !errors.Is(err, ErrBadFile) {
		t.Errorf("bad file: %v", err)
	}
	if Slug("Größe!") != "dhgre" || Slug("日本") != "" {
		t.Errorf("slug: %q %q", Slug("Größe!"), Slug("日本"))
	}
}
