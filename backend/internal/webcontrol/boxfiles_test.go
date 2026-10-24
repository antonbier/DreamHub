package webcontrol

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestParseFileList(t *testing.T) {
	cases := map[string][]string{
		`{"result":true,"files":["/etc/enigma2/netcastserver_b.json","netcastserver_a.json","/etc/enigma2/x.txt"]}`: {"netcastserver_a.json", "netcastserver_b.json"},
		`{"files":[{"path":"/etc/enigma2/netcastserver_c.json"},{"name":"netcastserver_d.json"},{"size":3}]}`:       {"netcastserver_c.json", "netcastserver_d.json"},
		`{"files":[]}`: nil,
	}
	for in, want := range cases {
		got, err := ParseFileList([]byte(in), "netcastserver_*.json")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v %v, want %v", in, got, err, want)
		}
	}
	if _, err := ParseFileList([]byte("<xml/>"), "*"); err == nil {
		t.Error("xml accepted")
	}
}

// The One's /web/mediaplayerlist (recorded 2026-10-24): the folder and the name
// are glued together without a slash, the folder itself is the first entry.
func TestParseMediaPlayerList_Dreambox(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<e2filelist>
	<e2file><e2servicereference>/</e2servicereference><e2isdirectory>True</e2isdirectory><e2root>/etc/enigma2</e2root></e2file>
	<e2file><e2servicereference>/etc/enigma2netcastserver_vavoo.json</e2servicereference><e2isdirectory>False</e2isdirectory><e2root>/etc/enigma2</e2root></e2file>
	<e2file><e2servicereference>/etc/enigma2netcastepg_gemini.json</e2servicereference><e2isdirectory>False</e2isdirectory><e2root>/etc/enigma2</e2root></e2file>
	<e2file><e2servicereference>/etc/enigma2netcastserver_gemini.json</e2servicereference><e2isdirectory>False</e2isdirectory><e2root>/etc/enigma2</e2root></e2file>
	<e2file><e2servicereference>/etc/enigma2geminibrowser.json</e2servicereference><e2isdirectory>False</e2isdirectory><e2root>/etc/enigma2</e2root></e2file>
</e2filelist>`)
	got, err := ParseMediaPlayerList(raw, "/etc/enigma2", "netcastserver_*.json")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"netcastserver_gemini.json", "netcastserver_vavoo.json"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// plugin files route: /dreamhub/files/{info,write}
func filesPlugin(t *testing.T, switchOn bool, hasRoute bool, got *url.Values, hdr *string) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasRoute {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/dreamhub/files/info":
			fmt.Fprintf(w, `{"ok": true, "api": 1, "writes": %v}`, switchOn)
		case "/dreamhub/files/write":
			r.ParseForm()
			*got, *hdr = r.PostForm, r.Header.Get("X-DreamHub-Request")
			if !switchOn {
				fmt.Fprint(w, `{"ok": false, "error": "writes_off", "message": "x"}`)
				return
			}
			if r.PostForm.Get("path") != "/etc/enigma2/netcastserver_dhx.json" {
				fmt.Fprint(w, `{"ok": false, "error": "bad_path", "message": "not a Netcast source list"}`)
				return
			}
			fmt.Fprint(w, `{"ok": true, "bytes": 2}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL)
}

func TestWriteBoxFile_ThroughThePlugin(t *testing.T) {
	var got url.Values
	var hdr string
	c := filesPlugin(t, true, true, &got, &hdr)
	if !c.CanWriteBoxFiles(context.Background()) {
		t.Fatal("the route is there and the switch is on")
	}
	if err := c.WriteBoxFile(context.Background(), "/etc/enigma2/netcastserver_dhx.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if hdr != "1" || got.Get("data") != "{}" {
		t.Fatalf("header %q, form %v", hdr, got)
	}
	var rej *RejectedError
	if err := c.WriteBoxFile(context.Background(), "/etc/passwd", []byte("{}")); !errors.As(err, &rej) || !strings.Contains(rej.Text, "bad_path") {
		t.Fatalf("a refusal of the plugin: %v", err)
	}
}

func TestWriteBoxFile_SwitchOffAndOldPlugin(t *testing.T) {
	var got url.Values
	var hdr string
	off := filesPlugin(t, false, true, &got, &hdr)
	if off.CanWriteBoxFiles(context.Background()) {
		t.Fatal("the switch is off")
	}
	if err := off.WriteBoxFile(context.Background(), "/etc/enigma2/netcastserver_dhx.json", []byte("{}")); !errors.Is(err, ErrWritesOff) {
		t.Fatalf("want ErrWritesOff, got %v", err)
	}
	if got != nil {
		t.Fatal("nothing is posted while the switch is off")
	}
	old := filesPlugin(t, true, false, &got, &hdr)
	if old.CanWriteBoxFiles(context.Background()) {
		t.Fatal("no route")
	}
	if err := old.WriteBoxFile(context.Background(), "/etc/enigma2/netcastserver_dhx.json", []byte("{}")); !errors.Is(err, ErrNeedsPlugin) {
		t.Fatalf("want ErrNeedsPlugin, got %v", err)
	}
}
