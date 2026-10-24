package iptv

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const guideSample = "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tv>\n" +
	"<channel id=\"Das.Erste.de\"><display-name lang=\"de\">Das Erste</display-name><display-name>ARD</display-name></channel>\n" +
	"<channel id=\"ZDF.de\"><display-name>ZDF</display-name></channel>\n" +
	"<programme start=\"20261024200000 +0200\" channel=\"ZDF.de\"><title>x</title></programme>\n" +
	"<channel id=\"late\"><display-name>Late</display-name></channel>\n</tv>"

func TestParseGuideChannels_StopsAtTheFirstProgramme(t *testing.T) {
	gc, err := ParseGuideChannels(strings.NewReader(guideSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(gc.Channels) != 2 || gc.Channels[0].ID != "Das.Erste.de" || len(gc.Channels[0].Names) != 2 {
		t.Fatalf("channels: %+v", gc.Channels)
	}
}

func TestParseGuideChannels_Gzip(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(guideSample))
	zw.Close()
	gc, err := ParseGuideChannels(&buf)
	if err != nil || len(gc.Channels) != 2 {
		t.Fatalf("%+v %v", gc, err)
	}
}

func TestParseGuideChannels_NotAGuide(t *testing.T) {
	for _, in := range []string{"<html><body>login</body></html>", "#EXTM3U\n", "", "<tv><channel id=\"a\""} {
		if _, err := ParseGuideChannels(strings.NewReader(in)); err == nil {
			if !strings.Contains(in, "<tv>") {
				t.Errorf("%q: no error", in)
			}
		}
	}
}

func TestParseRepo(t *testing.T) {
	cases := map[string]Repo{
		"https://github.com/iptv-org/epg":                {Owner: "iptv-org", Name: "epg"},
		"iptv-org/iptv":                                  {Owner: "iptv-org", Name: "iptv"},
		"https://github.com/a/b.git":                     {Owner: "a", Name: "b"},
		"https://github.com/a/b/tree/main/lists/de/":     {Owner: "a", Name: "b", Ref: "main", Dir: "lists/de"},
		"https://github.com/a/b/blob/dev/lists/de/x.m3u": {Owner: "a", Name: "b", Ref: "dev", Dir: "lists/de"},
		"https://github.com/a/b/blob/dev/x.m3u":          {Owner: "a", Name: "b", Ref: "dev"},
	}
	for in, want := range cases {
		got, ok := ParseRepo(in)
		if !ok || got != want {
			t.Errorf("%q: %+v %v, want %+v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "x", "https://gitlab.com/a/b", "https://github.com/a", "a/b/c", "../x", "a b/c"} {
		if _, ok := ParseRepo(in); ok {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestListRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/a/b/contents/lists" || r.URL.Query().Get("ref") != "main" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`[{"name":"de.xml.gz","path":"lists/de.xml.gz","type":"file","size":9},
			{"name":"README.md","path":"lists/README.md","type":"file"},
			{"name":"tv.m3u","path":"lists/tv.m3u","type":"file","size":3},
			{"name":"sub","path":"lists/sub","type":"dir"}]`))
	}))
	defer srv.Close()
	old := GitHubAPI
	GitHubAPI = srv.URL
	defer func() { GitHubAPI = old }()
	l, err := (&Fetcher{AllowLoopback: true}).ListRepo(context.Background(), Repo{Owner: "a", Name: "b", Ref: "main", Dir: "lists"})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Files) != 2 || l.Files[0].Kind != "guide" || l.Files[1].Kind != "playlist" || len(l.Dirs) != 1 ||
		l.Files[0].URL != "https://raw.githubusercontent.com/a/b/main/lists/de.xml.gz" {
		t.Errorf("listing: %+v", l)
	}
}
