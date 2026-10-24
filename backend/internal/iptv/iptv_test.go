package iptv

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sample = "\xef\xbb\xbf#EXTM3U x-tvg-url=\"http://epg.example/guide.xml\"\r\n" +
	"#EXTINF:-1 tvg-id=\"das-erste.de\" tvg-name=\"Das Erste\" tvg-logo=\"http://logos.example/erste.png\" group-title=\"Germany, Public\",Das Erste HD\r\n" +
	"http://tv.example/live/erste.m3u8\r\n" +
	"#EXTINF:-1 radio=\"true\" group-title=\"Radio\",Dlf Kultur\n" +
	"#EXTVLCOPT:http-user-agent=VLC\n" +
	"https://radio.example/dlf.mp3\n" +
	"#EXTGRP:Kids\n" +
	"#EXTINF:-1,Toon\n" +
	"rtsp://user:secret@cam.example:554/stream1\n" +
	"#EXTINF:-1,Local file\n" +
	"file:///tmp/x.ts\n" +
	"#EXTINF:-1,Spaces\n" +
	"http://tv.example/a b.m3u8\n" +
	"http://bare.example/path/channel7.ts\n"

func TestParse(t *testing.T) {
	pl, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Entries) != 4 || len(pl.Skipped) != 2 {
		t.Fatalf("entries %d, skipped %d: %+v", len(pl.Entries), len(pl.Skipped), pl)
	}
	e := pl.Entries[0]
	if e.Name != "Das Erste HD" || e.TvgID != "das-erste.de" || e.TvgName != "Das Erste" ||
		e.Logo != "http://logos.example/erste.png" || e.Group != "Germany, Public" || e.Radio {
		t.Errorf("first entry: %+v", e)
	}
	r := pl.Entries[1]
	if !r.Radio || r.Group != "Radio" || len(r.Options) != 1 {
		t.Errorf("radio entry: %+v", r)
	}
	if pl.Entries[2].Group != "Kids" || pl.Entries[2].Name != "Toon" {
		t.Errorf("EXTGRP entry: %+v", pl.Entries[2])
	}
	if pl.Entries[3].Name != "channel7.ts" {
		t.Errorf("a bare address takes its name from the path: %+v", pl.Entries[3])
	}
	reasons := pl.Skipped[0].Reason + "," + pl.Skipped[1].Reason
	if reasons != "scheme,invalid" {
		t.Errorf("skipped: %s", reasons)
	}
}

func TestParse_NotAPlaylist(t *testing.T) {
	for name, in := range map[string]string{
		"html":  "<html><body>Login</body></html>",
		"empty": "",
		"text":  "just some words\n",
	} {
		if _, err := Parse(strings.NewReader(in)); !errors.Is(err, ErrNotPlaylist) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestParse_Truncates(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for i := 0; i < MaxEntries+5; i++ {
		b.WriteString("http://h.example/")
		b.WriteString(strings.Repeat("a", i%7+1))
		b.WriteString("\n")
	}
	pl, err := Parse(strings.NewReader(b.String()))
	if err != nil || !pl.Truncated || len(pl.Entries) != MaxEntries {
		t.Fatalf("%v truncated=%v n=%d", err, pl != nil && pl.Truncated, len(pl.Entries))
	}
}

func TestLoginAndDisplay(t *testing.T) {
	for raw, login := range map[string]bool{
		"http://h.example/a.m3u8":                 false,
		"rtsp://user:secret@cam.example/s":        true,
		"http://h.example/get.php?username=a&x=1": true,
		"http://h.example/a?Password=1":           true,
		"http://h.example/a?token=abc":            true,
		"http://h.example/a?quality=hd&channel=7": false,
	} {
		if HasLogin(raw) != login {
			t.Errorf("HasLogin(%q) = %v", raw, !login)
		}
	}
	d := Display("http://user:secret@Cam.example:8080/live/a.m3u8?token=abc#x")
	if d != "http://Cam.example:8080/live/a.m3u8" {
		t.Errorf("Display: %q", d)
	}
	if strings.Contains(Display("rtsp://user:secret@cam.example/s"), "secret") {
		t.Error("Display leaks the login")
	}
}

func TestRef(t *testing.T) {
	ref := Ref("http://h.example:8080/a%20b.m3u8?x=1")
	if ref != "4097:0:1:0:0:0:0:0:0:0:http%3a//h.example%3a8080/a%2520b.m3u8?x=1" {
		t.Errorf("Ref: %q", ref)
	}
	// the box appends ":<name>"; the address is still found, with any name
	if URLPart(ref+":Das Erste: HD") != ref[len("4097:0:1:0:0:0:0:0:0:0:"):] {
		t.Errorf("URLPart: %q", URLPart(ref+":Das Erste: HD"))
	}
	if URLPart("1:0:19:283D:3FB:1:C00000:0:0:0:") != "" {
		t.Error("a DVB reference has no stream address")
	}
	if got := RedactRef(ref + ":Name"); strings.Contains(got, "h.example") || !strings.HasSuffix(got, ":<stream>") {
		t.Errorf("RedactRef: %q", got)
	}
}

func TestFetch(t *testing.T) {
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(sample))
	zw.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/plain.m3u", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sample)) })
	mux.HandleFunc("/packed.m3u.gz", func(w http.ResponseWriter, r *http.Request) { w.Write(gz.Bytes()) })
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) { w.Write(bytes.Repeat([]byte("#"), 2000)) })
	mux.HandleFunc("/hop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/plain.m3u", http.StatusFound) })
	mux.HandleFunc("/leave", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "ftp://x.example/a", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := &Fetcher{AllowLoopback: true, MaxBytes: 1000 + int64(len(sample))}
	for _, path := range []string{"/plain.m3u", "/packed.m3u.gz", "/hop"} {
		body, err := f.Fetch(context.Background(), srv.URL+path)
		if err != nil || string(body) != sample {
			t.Errorf("%s: %v", path, err)
		}
	}
	for path, kind := range map[string]string{"/missing": KindStatus, "/big": KindTooLarge, "/leave": KindInvalidURL} {
		_, err := f.Fetch(context.Background(), srv.URL+path)
		var fe *FetchError
		if !errors.As(err, &fe) || fe.Kind != kind {
			t.Errorf("%s: %v, want %s", path, err, kind)
		}
	}
	if _, err := f.Fetch(context.Background(), "ftp://h.example/a.m3u"); err == nil {
		t.Error("only http and https are fetched")
	}
}

func TestFetch_BlocksLoopbackAndHidesTheAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sample)) }))
	defer srv.Close()
	addr := strings.Replace(srv.URL, "http://", "http://user:secret@", 1) + "/get.php?password=hunter2"
	_, err := NewFetcher().Fetch(context.Background(), addr)
	var fe *FetchError
	if !errors.As(err, &fe) || fe.Kind != KindBlocked {
		t.Fatalf("a server on this machine must be refused: %v", err)
	}
	for _, leak := range []string{"secret", "hunter2", "127.0.0.1"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("the error holds %q: %v", leak, err)
		}
	}
}

func TestUploadIsLimitedAndUnpacked(t *testing.T) {
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(sample))
	zw.Close()
	f := &Fetcher{MaxBytes: 4096}
	if body, err := f.ReadUpload(bytes.NewReader(gz.Bytes())); err != nil || string(body) != sample {
		t.Errorf("gzip upload: %v", err)
	}
	if _, err := f.ReadUpload(bytes.NewReader(bytes.Repeat([]byte("a"), 5000))); err == nil {
		t.Error("an upload over the limit is refused")
	}
}
