package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The signatures are the examples in eWeLink's own documentation.
func TestSign_DocumentationExamples(t *testing.T) {
	if got := sign("abc", "ABC_123"); got != "v1+mfNY2ukxswM8sZOTg99srZsVnUVv9DGXeav1096M=" {
		t.Errorf("login signature: %s", got)
	}
	body := `{"email":"1234@gmail.com","password":"12345678","countryCode":"+1"}`
	if got := sign("OdPuCZ4PkPPi0rVKRVcGmll2NM6vVk0c", body); got != "ttZ/gluzqrafvGonjMD20p4//arW6KoZKbo1SOMEzCA=" {
		t.Errorf("body signature: %s", got)
	}
}

func TestParseKwh(t *testing.T) {
	days := ParseKwh("190101190301000000")
	if len(days) != 3 || days[0] != 25.11 || days[1] != 25.31 || days[2] != 0 {
		t.Fatalf("%v", days)
	}
	if got := monthTotal([]float64{1, 2, 3, 4}, time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)); got != 6 {
		t.Errorf("third of the month: %v", got)
	}
	if got := monthTotal([]float64{1, 2}, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)); got != 3 {
		t.Errorf("short history: %v", got)
	}
}

func TestParseLoginRedirect(t *testing.T) {
	for in, want := range map[string][2]string{
		"https://dreamhub.nimra89.de/?code=95bc-41b&region=eu&state=dreamhub": {"95bc-41b", "eu"},
		"?code=abc&region=as": {"abc", "as"},
		"code=abc":            {"abc", ""},
		"  95bcf41b-3397  ":   {"95bcf41b-3397", ""},
	} {
		code, region, err := ParseLoginRedirect(in)
		if err != nil || code != want[0] || region != want[1] {
			t.Errorf("%q: %q %q %v", in, code, region, err)
		}
	}
	if _, _, err := ParseLoginRedirect("https://x.de/?error=1"); err == nil {
		t.Error("a redirect without a code was accepted")
	}
}

func TestLoginURL(t *testing.T) {
	cl := &Cloud{Now: func() time.Time { return time.UnixMilli(123) }}
	u := cl.LoginURL(CloudConfig{AppID: "ABC", Secret: "abc", RedirectURL: "https://dreamhub.nimra89.de"})
	for _, want := range []string{"clientId=ABC", "seq=123", "authorization=v1%2BmfNY2ukxswM8sZOTg99srZsVnUVv9DGXeav1096M%3D", "grantType=authorization_code", "redirectUrl=https%3A%2F%2Fdreamhub.nimra89.de"} {
		if !strings.Contains(u, want) {
			t.Errorf("login link lacks %s: %s", want, u)
		}
	}
}

// fakeCloud answers the calls the extension makes.
func fakeCloud(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		reply := func(data any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": 0, "msg": "", "data": data})
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /v2/user/oauth/token":
			if r.Header.Get("X-CK-Appid") != "APP" || r.Header.Get("Authorization") != "Sign "+sign("SECRET", string(raw)) {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": 401, "msg": "bad signature", "data": map[string]any{}})
				return
			}
			var b map[string]string
			_ = json.Unmarshal(raw, &b)
			if b["code"] != "CODE" || b["redirectUrl"] != "https://r.example" || b["grantType"] != "authorization_code" {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": 405, "msg": "invalid code", "data": map[string]any{}})
				return
			}
			reply(map[string]any{"accessToken": "AT", "atExpiredTime": time.Now().Add(30 * 24 * time.Hour).UnixMilli(), "refreshToken": "RT", "rtExpiredTime": time.Now().Add(60 * 24 * time.Hour).UnixMilli()})
		case "GET /v2/device/thing":
			if r.Header.Get("Authorization") != "Bearer AT" {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": 401, "msg": "token", "data": map[string]any{}})
				return
			}
			reply(map[string]any{"total": 3, "thingList": []any{
				map[string]any{"itemType": 1, "itemData": map[string]any{"deviceid": "1002a17548", "name": "Unraid plug", "devicekey": "KEY-1"}},
				map[string]any{"itemType": 1, "itemData": map[string]any{"deviceid": "other", "name": "Lamp", "devicekey": "KEY-2"}},
				map[string]any{"itemType": 2, "itemData": map[string]any{"deviceid": "shared", "name": "Shared", "devicekey": "KEY-3"}},
			}})
		case "POST /v2/device/thing/status":
			reply(map[string]any{})
		case "GET /v2/device/thing/status":
			reply(map[string]any{"params": map[string]any{"power": "212.50", "voltage": "231.0", "hundredDaysKwhData": "010002030000050000"}})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestCloud_ExchangeThingsAndEnergy(t *testing.T) {
	kwhWait = 0
	var calls []string
	srv := fakeCloud(t, &calls)
	defer srv.Close()
	cl := &Cloud{Base: srv.URL, Client: srv.Client(), Now: func() time.Time { return time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC) }}
	cfg := CloudConfig{AppID: "APP", Secret: "SECRET", RedirectURL: "https://r.example"}

	if _, err := cl.Exchange(context.Background(), cfg, "WRONG", ""); err == nil || !strings.Contains(err.Error(), "invalid code") {
		t.Fatalf("a wrong code: %v", err)
	}
	cfg, err := cl.Exchange(context.Background(), cfg, "CODE", "eu")
	if err != nil || cfg.AccessToken != "AT" || cfg.RefreshToken != "RT" {
		t.Fatalf("exchange: %+v %v", cfg, err)
	}
	things, err := cl.Things(context.Background(), cfg)
	if err != nil || len(things) != 2 || things[0].Key != "KEY-1" {
		t.Fatalf("things (own devices only): %+v %v", things, err)
	}
	e := readEnergy(context.Background(), cl, cfg, "1002a17548", cl.Now(), true)
	if e.LastError != "" || e.PowerW == nil || *e.PowerW != 212.5 {
		t.Fatalf("energy: %+v", e)
	}
	// 1.02 today, 3.00 yesterday, 0.05... : the 2nd of the month is two days.
	if e.TodayKwh == nil || *e.TodayKwh != 1.02 || e.MonthKwh == nil || *e.MonthKwh != 1.02+3.00 {
		t.Fatalf("consumption: today %v month %v", e.TodayKwh, e.MonthKwh)
	}
}

func TestFresh_RefreshesWithinADay(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/user/refresh" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"error": 0, "data": map[string]any{"at": "AT2", "rt": "RT2"}})
	}))
	defer srv.Close()
	cl := &Cloud{Base: srv.URL, Client: srv.Client(), Now: func() time.Time { return now }}
	good := CloudConfig{AccessToken: "AT", AtExpires: now.Add(10 * 24 * time.Hour).UnixMilli(), RefreshToken: "RT"}
	if out, changed, err := cl.Fresh(context.Background(), good); err != nil || changed || out.AccessToken != "AT" {
		t.Fatalf("a token with days left was touched: %+v %v %v", out, changed, err)
	}
	old := good
	old.AtExpires = now.Add(time.Hour).UnixMilli()
	out, changed, err := cl.Fresh(context.Background(), old)
	if err != nil || !changed || out.AccessToken != "AT2" || out.RefreshToken != "RT2" {
		t.Fatalf("refresh: %+v %v %v", out, changed, err)
	}
}

// The settings form: save the application, get a login link, paste the
// address, and the key of a device on the network lands in its settings.
func TestSettings_LoginFetchesKeys(t *testing.T) {
	var calls []string
	srv := fakeCloud(t, &calls)
	defer srv.Close()
	store := NewConfigStore("", Config{Lang: "en"})
	devices := NewDevices(store)
	devices.list["1002a17548"] = Device{ID: "1002a17548", Type: "plug", Seen: time.Now()}
	s := &Server{cfg: store, devices: devices, cloud: &Cloud{Base: srv.URL, Client: srv.Client(), Now: time.Now}, energy: NewEnergyStore()}
	h := s.Handler()

	put := func(values map[string]string) (int, string) {
		b, _ := json.Marshal(map[string]any{"values": values})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/settings", strings.NewReader(string(b))))
		return rec.Code, rec.Body.String()
	}
	if code, body := put(map[string]string{"cloud:appId": "APP", "cloud:secret": "SECRET", "cloud:redirectUrl": "https://r.example", "cloud:region": "eu"}); code != 200 || !strings.Contains(body, `"type":"link"`) {
		t.Fatalf("the login link didn't appear: %d %s", code, body)
	}
	if store.Get().Cloud.Secret != "SECRET" {
		t.Fatal("the secret wasn't stored")
	}
	if code, body := put(map[string]string{"cloud:code": "https://r.example/?code=WRONG&region=eu&state=dreamhub"}); code != 400 || !strings.Contains(body, "invalid code") {
		t.Fatalf("a wrong code: %d %s", code, body)
	}
	if code, body := put(map[string]string{"cloud:code": "https://r.example/?code=CODE&region=eu&state=dreamhub"}); code != 200 {
		t.Fatalf("login: %d %s", code, body)
	}
	cfg := store.Get()
	d, ok := cfg.device("1002a17548")
	if !ok || d.Key != "KEY-1" || d.Name != "Unraid plug" {
		t.Fatalf("the key of the device on the network: %+v", cfg.Devices)
	}
	if _, ok := cfg.device("other"); ok {
		t.Fatal("a device that is not on the network was added")
	}
	if !cfg.Cloud.LoggedIn() {
		t.Fatal("the login wasn't kept")
	}
	// The secret never comes back.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if strings.Contains(rec.Body.String(), "SECRET") || strings.Contains(rec.Body.String(), "KEY-1") || strings.Contains(rec.Body.String(), `"AT"`) {
		t.Fatal("a secret was sent back to the browser")
	}
}

func TestWidgets_TileFlagsAndConsumption(t *testing.T) {
	store := NewConfigStore("", Config{Lang: "en", Devices: []DeviceConfig{{ID: "p1", Key: "k", Name: "Plug"}}})
	devices := NewDevices(store)
	devices.list["p1"] = Device{ID: "p1", State: map[string]any{"switch": "on", "power": "12.3"}, Seen: time.Now()}
	energy := NewEnergyStore()
	energy.set("p1", Energy{TodayKwh: ptr(1.5), MonthKwh: ptr(20.25)})
	devices.list["p1"] = Device{ID: "p1", State: map[string]any{"switch": "on", "power": "12.3", "monthKwh": float64(90)}, Seen: time.Now()}
	s := &Server{cfg: store, devices: devices, cloud: NewCloud(), energy: energy}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/widgets", nil))
	var ws []Widget
	if err := json.Unmarshal(rec.Body.Bytes(), &ws); err != nil {
		t.Fatal(err)
	}
	var tile []string
	for _, w := range ws {
		if w.Tile {
			tile = append(tile, w.Type+":"+w.Label)
		}
		if w.Type == "action" && w.Tile {
			t.Error("an action was flagged for the start page")
		}
	}
	if len(tile) != 2 || tile[1] != "list:Plug · Consumption" {
		t.Fatalf("tile widgets: %v", tile)
	}
	body := rec.Body.String()
	for _, want := range []string{"12.3 W", "1.50 kWh", "90.00 kWh"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
}
