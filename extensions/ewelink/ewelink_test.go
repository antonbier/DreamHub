package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The real answer of the owner's Sonoff S60TPF (captured on the LAN
// 2026-10-12, LAN control on).
func TestParseResponse_RealS60TPF(t *testing.T) {
	b, err := os.ReadFile("testdata/s60tpf-mdns.b64")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	anns := ParseResponse(msg)
	if len(anns) != 1 {
		t.Fatalf("got %d announcements", len(anns))
	}
	a := anns[0]
	if a.TXT["id"] != "1002a17548" || a.TXT["type"] != "plug" || a.TXT["encrypt"] != "true" || a.TXT["iv"] == "" || a.TXT["data1"] == "" {
		t.Fatalf("txt: %v", a.TXT)
	}
	if a.Port != 8081 || a.IP.String() != "192.168.1.64" {
		t.Fatalf("addr: %v:%d", a.IP, a.Port)
	}
	// Without the key the state can't be read, and says so.
	if _, err := DecryptTXT(a.TXT, ""); err != errNoKey {
		t.Fatalf("got %v", err)
	}
	if _, err := DecryptTXT(a.TXT, "wrong-key"); err == nil {
		t.Fatal("a wrong key must not yield a state")
	}
}

// A vector made with openssl, independent of this code: key = MD5 of the
// device key, AES-128-CBC, PKCS#7.
func TestDecrypt_OpensslVector(t *testing.T) {
	txt := map[string]string{"encrypt": "true", "iv": "AAECAwQFBgcICQoLDA0ODw==", "data1": "iEL7eu7sLfxmC3p+9OB4dw=="}
	state, err := DecryptTXT(txt, "testkey123")
	if err != nil || state["switch"] != "on" {
		t.Fatalf("got %v %v", state, err)
	}
}

func TestEncryptRoundTrip(t *testing.T) {
	iv, data, err := Encrypt("k", []byte(`{"switch":"off"}`))
	if err != nil {
		t.Fatal(err)
	}
	state, err := DecryptTXT(map[string]string{"encrypt": "true", "iv": iv, "data1": data}, "k")
	if err != nil || state["switch"] != "off" {
		t.Fatalf("got %v %v", state, err)
	}
}

// A fake plug: it decrypts the command with the key, and the next browse
// announces the new state.
func TestSwitchAgainstFakeDevice(t *testing.T) {
	const key = "devkey"
	state := "on"
	seq := 1
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Deviceid, Iv, Data string }
		json.NewDecoder(r.Body).Decode(&body)
		got, err := DecryptTXT(map[string]string{"encrypt": "true", "iv": body.Iv, "data1": body.Data}, key)
		if r.URL.Path != "/zeroconf/switch" || err != nil || body.Deviceid != "abc" {
			w.Write([]byte(`{"error":400}`))
			return
		}
		state = got["switch"].(string)
		seq++
		w.Write([]byte(`{"seq":2,"error":0}`))
	}))
	defer dev.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(dev.URL, "http://"))

	cfg := Config{Lang: "de", Devices: []DeviceConfig{{ID: "abc", Key: key, Name: "Unraid", OffWarning: "Vorher herunterfahren."}}}
	store := NewConfigStore("", cfg)
	ds := NewDevices(store)
	ds.browse = func(context.Context, time.Duration) ([]Announcement, error) {
		iv, data, _ := Encrypt(key, []byte(`{"switch":"`+state+`","power":"12.5"}`))
		p := 0
		for _, c := range port {
			p = p*10 + int(c-'0')
		}
		return []Announcement{{IP: net.ParseIP(host), Port: uint16(p), TXT: map[string]string{
			"id": "abc", "type": "plug", "encrypt": "true", "iv": iv, "data1": data, "seq": string(rune('0' + seq))}}}, nil
	}
	if err := ds.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer((&Server{cfg: store, devices: ds}).Handler())
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/widgets")
	var widgets []Widget
	json.NewDecoder(resp.Body).Decode(&widgets)
	if len(widgets) != 3 || widgets[0].Value != "An" || widgets[1].Type != "list" || widgets[2].ButtonLabel != "Ausschalten" {
		t.Fatalf("widgets: %+v", widgets)
	}

	resp, _ = http.Post(srv.URL+"/actions/toggle-abc/plan", "application/json", nil)
	var prev struct{ Summary, Fingerprint string }
	json.NewDecoder(resp.Body).Decode(&prev)
	if !strings.Contains(prev.Summary, "ausgeschaltet") || !strings.Contains(prev.Summary, "Vorher herunterfahren") {
		t.Fatalf("preview: %+v", prev)
	}

	// A stale fingerprint is refused and nothing is switched.
	resp, _ = http.Post(srv.URL+"/actions/toggle-abc/apply", "application/json", strings.NewReader(`{"fingerprint":"old"}`))
	if resp.StatusCode != http.StatusConflict || state != "on" {
		t.Fatalf("stale apply: %d, state %s", resp.StatusCode, state)
	}

	body, _ := json.Marshal(map[string]string{"fingerprint": prev.Fingerprint})
	resp, _ = http.Post(srv.URL+"/actions/toggle-abc/apply", "application/json", strings.NewReader(string(body)))
	if resp.StatusCode != http.StatusOK || state != "off" {
		t.Fatalf("apply: %d, state %s", resp.StatusCode, state)
	}
}

func TestWidgets_UnconfiguredAndMissing(t *testing.T) {
	cfg := Config{Lang: "en", Devices: []DeviceConfig{{ID: "gone", Name: "Lamp"}}}
	store := NewConfigStore("", cfg)
	ds := NewDevices(store)
	ds.browse = func(context.Context, time.Duration) ([]Announcement, error) {
		return []Announcement{{IP: net.IPv4(192, 168, 1, 64), TXT: map[string]string{"id": "1002a17548", "encrypt": "true"}}}, nil
	}
	ds.Refresh(context.Background())
	rec := httptest.NewRecorder()
	(&Server{cfg: store, devices: ds}).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/widgets", nil))
	if !strings.Contains(rec.Body.String(), "not set up yet") || !strings.Contains(rec.Body.String(), "Not found on the network") {
		t.Fatal(rec.Body.String())
	}
}

func TestSettings_SaveKeyKeepsSecretAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewConfigStore(path, Config{Lang: "de"})
	srv := &Server{cfg: store, devices: NewDevices(store)}
	h := srv.Handler()

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("PUT", "/settings", strings.NewReader(body)))
		return rec
	}
	if rec := put(`{"values":{"key:abc":"s3cret","name:abc":"Unraid"}}`); rec.Code != 200 || strings.Contains(rec.Body.String(), "s3cret") {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if rec := put(`{"values":{"key:abc":""}}`); rec.Code != 200 {
		t.Fatalf("empty secret: %d", rec.Code)
	}
	onDisk, err := loadConfig(path)
	if err != nil || len(onDisk.Devices) != 1 || onDisk.Devices[0].Key != "s3cret" || onDisk.Devices[0].Name != "Unraid" {
		t.Fatalf("file: %+v %v", onDisk, err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	if rec := put(`{"values":{"lang":"fr"}}`); rec.Code != 400 {
		t.Fatalf("bad language: %d", rec.Code)
	}
}

// A device that announces "switches" (the S60TPF) takes /zeroconf/switches
// with the channel listed; the single-channel call is refused with 400.
func TestSwitchMultiChannelUsesSwitchesEndpoint(t *testing.T) {
	const key = "devkey"
	state := "off"
	var path string
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Deviceid, Iv, Data string }
		json.NewDecoder(r.Body).Decode(&body)
		path = r.URL.Path
		got, err := DecryptTXT(map[string]string{"encrypt": "true", "iv": body.Iv, "data1": body.Data}, key)
		list, _ := got["switches"].([]any)
		if r.URL.Path != "/zeroconf/switches" || err != nil || len(list) != 1 {
			w.Write([]byte(`{"error":400}`))
			return
		}
		state = list[0].(map[string]any)["switch"].(string)
		w.Write([]byte(`{"error":0}`))
	}))
	defer dev.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(dev.URL, "http://"))
	p, _ := strconv.Atoi(port)
	ds := NewDevices(NewConfigStore("", Config{Devices: []DeviceConfig{{ID: "abc", Key: key}}}))
	ds.browse = func(context.Context, time.Duration) ([]Announcement, error) {
		iv, data, _ := Encrypt(key, []byte(`{"switches":[{"switch":"`+state+`","outlet":0}]}`))
		return []Announcement{{IP: net.ParseIP(host), Port: uint16(p), TXT: map[string]string{
			"id": "abc", "type": "plug", "encrypt": "true", "iv": iv, "data1": data, "seq": "1"}}}, nil
	}
	if err := ds.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ds.Switch(context.Background(), "abc", true); err != nil || state != "on" || path != "/zeroconf/switches" {
		t.Fatalf("switch: %v, state %s, path %s", err, state, path)
	}
}
