package api_test

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

const zdfRef = "1:0:19:2B66:3F3:1:C00000:0:0:0:"

func TestStream_Live(t *testing.T) {
	box := mock.New()
	e := newEnvWith(t, box)

	resp, b := e.do(t, "GET", "/api/v1/stream/live?ref="+url.QueryEscape("4097:0:1:0:0:0:0:0:0:0:http%3a//evil/x"), "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusBadRequest || code != "invalid_query" {
		t.Errorf("only broadcast references: %d %s", resp.StatusCode, b)
	}
	resp, b = e.do(t, "GET", "/api/v1/stream/live?ref="+url.QueryEscape(zdfRef), "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusServiceUnavailable || code != "stream_no_tuner" {
		t.Errorf("a box that doesn't stream: %d %s", resp.StatusCode, b)
	}

	ts := bytes.Repeat(append([]byte{0x47}, make([]byte, 187)...), 50)
	file := filepath.Join(t.TempDir(), "live.ts")
	os.WriteFile(file, ts, 0o600)
	box.StreamFile = file
	resp, b = e.do(t, "GET", "/api/v1/stream/live?ref="+url.QueryEscape(zdfRef), "", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "video/mp2t" || !bytes.Equal(b, ts) || box.LastStreamed != zdfRef {
		t.Errorf("the box's stream, passed through: %d %q %d bytes, asked for %q", resp.StatusCode, resp.Header.Get("Content-Type"), len(b), box.LastStreamed)
	}

	// The stream proxy answers but sends nothing: no tuner.
	os.WriteFile(file, nil, 0o600)
	resp, b = e.do(t, "GET", "/api/v1/stream/live?ref="+url.QueryEscape(zdfRef), "", nil)
	if code, _, _ := errCode(t, b); resp.StatusCode != http.StatusServiceUnavailable || code != "stream_no_tuner" {
		t.Errorf("an empty stream means no tuner: %d %s", resp.StatusCode, b)
	}
}

func TestStream_Recording(t *testing.T) {
	box := mock.New()
	rec := mock.Root + "20260101 2015 - ZDF HD - Film.ts"
	content := strings.Repeat("0123456789", 100)
	box.FS.Put(rec, content)
	box.FS.Put(mock.Root+"notes.txt", "not a recording")
	e := newEnvWith(t, box)

	resp, b := e.do(t, "GET", "/api/v1/stream/recording?path="+url.QueryEscape(rec), "", nil)
	if resp.StatusCode != http.StatusOK || string(b) != content || resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("whole recording: %d %d bytes %q", resp.StatusCode, len(b), resp.Header.Get("Accept-Ranges"))
	}
	resp, b = e.do(t, "GET", "/api/v1/stream/recording?path="+url.QueryEscape(rec), "", func(r *http.Request) { r.Header.Set("Range", "bytes=500-509") })
	if resp.StatusCode != http.StatusPartialContent || string(b) != "0123456789" || resp.Header.Get("Content-Range") != "bytes 500-509/1000" {
		t.Errorf("seeking by range: %d %q %q", resp.StatusCode, b, resp.Header.Get("Content-Range"))
	}
	for _, p := range []string{mock.Root + "notes.txt", "/etc/passwd", mock.Root + "../../../etc/x.ts", mock.Root + "missing.ts"} {
		if resp, _ := e.do(t, "GET", "/api/v1/stream/recording?path="+url.QueryEscape(p), "", nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", p, resp.StatusCode)
		}
	}
}

func TestStream_Channels(t *testing.T) {
	e := newEnv(t)
	_, b := e.do(t, "GET", "/api/v1/stream/bouquets", "", nil)
	if !strings.Contains(string(b), `"bouquets":[{`) {
		t.Fatalf("bouquets: %s", b)
	}
	resp, b := e.do(t, "GET", "/api/v1/stream/channels?bouquet="+url.QueryEscape(`1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.favourites.tv" ORDER BY bouquet`), "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"channels":[{`) || !strings.Contains(string(b), `"playing"`) {
		t.Errorf("channels with now/next: %d %s", resp.StatusCode, b)
	}
}
