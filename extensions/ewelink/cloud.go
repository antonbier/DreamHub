package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The eWeLink cloud is used for two things only, both read-only:
//
//  1. fetching each device's devicekey once (the key that unlocks the LAN
//     protocol, which lives only in the owner's account), and
//  2. reading the plug's consumption history (hundredDaysKwh), which the
//     LAN protocol doesn't offer.
//
// Switching and the live state never go through the cloud. The OAuth 2.0
// flow of the owner's own developer application (dev.ewelink.cc, "App
// Management") is used: the owner logs in on eWeLink's page, which sends
// the browser to the redirect URL with a one-time code (30 s); the code is
// pasted into the settings form. https://coolkit-technologies.github.io/eWeLink-API/

// CloudConfig is the developer application and what logging in gave.
type CloudConfig struct {
	AppID       string `json:"appId"`
	Secret      string `json:"secret"`
	Region      string `json:"region"` // eu, us, as, cn
	RedirectURL string `json:"redirectUrl"`

	AccessToken  string `json:"accessToken,omitempty"`
	AtExpires    int64  `json:"atExpires,omitempty"` // ms since the epoch
	RefreshToken string `json:"refreshToken,omitempty"`
	RtExpires    int64  `json:"rtExpires,omitempty"`
}

func (c CloudConfig) region() string {
	switch c.Region {
	case "us", "as", "cn":
		return c.Region
	}
	return "eu"
}

// Ready says the application is filled in, so a login link can be made.
func (c CloudConfig) Ready() bool { return c.AppID != "" && c.Secret != "" && c.RedirectURL != "" }

// LoggedIn says a token is stored (it may need a refresh).
func (c CloudConfig) LoggedIn() bool { return c.RefreshToken != "" || c.AccessToken != "" }

const loginPage = "https://c2ccdn.coolkit.cc/oauth/index.html"

var apiHosts = map[string]string{
	"eu": "https://eu-apia.coolkit.cc",
	"us": "https://us-apia.coolkit.cc",
	"as": "https://as-apia.coolkit.cc",
	"cn": "https://cn-apia.coolkit.cn",
}

// Cloud talks to the eWeLink v2 API.
type Cloud struct {
	// Base overrides the regional host (tests).
	Base   string
	Client *http.Client
	Now    func() time.Time
}

func NewCloud() *Cloud {
	return &Cloud{Client: &http.Client{Timeout: 20 * time.Second}, Now: time.Now}
}

func (c *Cloud) base(region string) string {
	if c.Base != "" {
		return c.Base
	}
	return apiHosts[region]
}

func sign(secret, message string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

func nonce() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}

// LoginURL is eWeLink's authorization page for the owner's application.
func (c *Cloud) LoginURL(cfg CloudConfig) string {
	seq := strconv.FormatInt(c.Now().UnixMilli(), 10)
	q := url.Values{}
	q.Set("clientId", cfg.AppID)
	q.Set("seq", seq)
	q.Set("authorization", sign(cfg.Secret, cfg.AppID+"_"+seq))
	q.Set("redirectUrl", cfg.RedirectURL)
	q.Set("grantType", "authorization_code")
	q.Set("state", "dreamhub")
	q.Set("nonce", nonce())
	q.Set("showQRCode", "false")
	return loginPage + "?" + q.Encode()
}

// ParseLoginRedirect takes what the person pasted — the whole address the
// browser landed on, a query string, or just the code — and returns the
// code and, when it is there, the region.
func ParseLoginRedirect(s string) (code, region string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", errors.New("nothing pasted")
	}
	if i := strings.Index(s, "?"); i >= 0 {
		s = s[i+1:]
	}
	if strings.Contains(s, "=") {
		q, perr := url.ParseQuery(strings.TrimPrefix(s, "&"))
		if perr != nil || q.Get("code") == "" {
			return "", "", errors.New("no code in what was pasted — copy the whole address after logging in")
		}
		return q.Get("code"), q.Get("region"), nil
	}
	return s, "", nil
}

// CloudError is the API's own refusal.
type CloudError struct {
	Code int
	Msg  string
}

func (e *CloudError) Error() string { return fmt.Sprintf("eWeLink answered %d: %s", e.Code, e.Msg) }

// call performs one request and unwraps {"error","msg","data"}.
func (c *Cloud) call(ctx context.Context, cfg CloudConfig, method, path string, query url.Values, body []byte, auth string) (json.RawMessage, error) {
	u := c.base(cfg.region()) + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-CK-Appid", cfg.AppID)
	req.Header.Set("X-CK-Nonce", nonce())
	req.Header.Set("Authorization", auth)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var env struct {
		Error int             `json:"error"`
		Msg   string          `json:"msg"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("eWeLink answered %d with something unreadable", resp.StatusCode)
	}
	if env.Error != 0 {
		return nil, &CloudError{Code: env.Error, Msg: env.Msg}
	}
	return env.Data, nil
}

// Exchange trades the login code for tokens (POST /v2/user/oauth/token).
// region is the one eWeLink sent back with the code, if it did.
func (c *Cloud) Exchange(ctx context.Context, cfg CloudConfig, code, region string) (CloudConfig, error) {
	if region != "" {
		cfg.Region = region
	}
	body, _ := json.Marshal(map[string]string{"code": code, "redirectUrl": cfg.RedirectURL, "grantType": "authorization_code"})
	data, err := c.call(ctx, cfg, http.MethodPost, "/v2/user/oauth/token", nil, body, "Sign "+sign(cfg.Secret, string(body)))
	if err != nil {
		return cfg, err
	}
	var tok struct {
		AccessToken   string `json:"accessToken"`
		AtExpiredTime int64  `json:"atExpiredTime"`
		RefreshToken  string `json:"refreshToken"`
		RtExpiredTime int64  `json:"rtExpiredTime"`
	}
	if err := json.Unmarshal(data, &tok); err != nil || tok.AccessToken == "" {
		return cfg, errors.New("eWeLink sent no access token")
	}
	cfg.AccessToken, cfg.AtExpires, cfg.RefreshToken, cfg.RtExpires = tok.AccessToken, tok.AtExpiredTime, tok.RefreshToken, tok.RtExpiredTime
	return cfg, nil
}

// Fresh returns cfg with a usable access token, refreshing it when it
// ends within a day (POST /v2/user/refresh). changed says tokens moved
// and cfg should be stored.
func (c *Cloud) Fresh(ctx context.Context, cfg CloudConfig) (out CloudConfig, changed bool, err error) {
	if cfg.AccessToken == "" && cfg.RefreshToken == "" {
		return cfg, false, errors.New("not logged in to eWeLink")
	}
	if cfg.AccessToken != "" && cfg.AtExpires > c.Now().Add(24*time.Hour).UnixMilli() {
		return cfg, false, nil
	}
	if cfg.RefreshToken == "" {
		return cfg, false, errors.New("the eWeLink login ran out — log in again")
	}
	body, _ := json.Marshal(map[string]string{"rt": cfg.RefreshToken})
	data, err := c.call(ctx, cfg, http.MethodPost, "/v2/user/refresh", nil, body, "Bearer "+cfg.AccessToken)
	if err != nil {
		return cfg, false, err
	}
	var tok struct {
		AT string `json:"at"`
		RT string `json:"rt"`
	}
	if err := json.Unmarshal(data, &tok); err != nil || tok.AT == "" {
		return cfg, false, errors.New("eWeLink sent no new token")
	}
	now := c.Now()
	cfg.AccessToken, cfg.AtExpires = tok.AT, now.Add(29*24*time.Hour).UnixMilli()
	if tok.RT != "" {
		cfg.RefreshToken, cfg.RtExpires = tok.RT, now.Add(59*24*time.Hour).UnixMilli()
	}
	return cfg, true, nil
}

// CloudDevice is what GET /v2/device/thing says about one device.
type CloudDevice struct {
	ID   string
	Name string
	Key  string
}

// Things lists the account's own devices with their keys.
func (c *Cloud) Things(ctx context.Context, cfg CloudConfig) ([]CloudDevice, error) {
	q := url.Values{"num": {"0"}}
	data, err := c.call(ctx, cfg, http.MethodGet, "/v2/device/thing", q, nil, "Bearer "+cfg.AccessToken)
	if err != nil {
		return nil, err
	}
	var d struct {
		ThingList []struct {
			ItemType int `json:"itemType"`
			ItemData struct {
				DeviceID  string `json:"deviceid"`
				Name      string `json:"name"`
				DeviceKey string `json:"devicekey"`
			} `json:"itemData"`
		} `json:"thingList"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, errors.New("eWeLink's device list was unreadable")
	}
	var out []CloudDevice
	for _, t := range d.ThingList {
		if t.ItemType != 1 || t.ItemData.DeviceID == "" { // own devices only
			continue
		}
		out = append(out, CloudDevice{ID: t.ItemData.DeviceID, Name: t.ItemData.Name, Key: t.ItemData.DeviceKey})
	}
	return out, nil
}

// Params reads a device's cloud state (GET /v2/device/thing/status).
func (c *Cloud) Params(ctx context.Context, cfg CloudConfig, id string, names ...string) (map[string]any, error) {
	q := url.Values{"type": {"1"}, "id": {id}}
	if len(names) > 0 {
		q.Set("params", strings.Join(names, "|"))
	}
	data, err := c.call(ctx, cfg, http.MethodGet, "/v2/device/thing/status", q, nil, "Bearer "+cfg.AccessToken)
	if err != nil {
		return nil, err
	}
	var d struct {
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, errors.New("eWeLink's device state was unreadable")
	}
	return d.Params, nil
}

// AskKwh asks the device to report its consumption history; the answer
// shows up in Params as hundredDaysKwhData a moment later.
func (c *Cloud) AskKwh(ctx context.Context, cfg CloudConfig, id string) error {
	body, _ := json.Marshal(map[string]any{"type": 1, "id": id, "params": map[string]string{"hundredDaysKwh": "get"}})
	_, err := c.call(ctx, cfg, http.MethodPost, "/v2/device/thing/status", nil, body, "Bearer "+cfg.AccessToken)
	return err
}

// ParseKwh decodes hundredDaysKwhData: 100 days, today first, three bytes
// each — the whole kWh, then the tenths and the hundredths as one digit
// each ("190101" is 25.11: 0x19, 1, 1). A short or odd string gives what
// it can.
func ParseKwh(s string) []float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	var days []float64
	for i := 0; i+6 <= len(s); i += 6 {
		whole, e1 := strconv.ParseUint(s[i:i+2], 16, 16)
		tenth, e2 := strconv.ParseUint(s[i+2:i+4], 16, 16)
		hundredth, e3 := strconv.ParseUint(s[i+4:i+6], 16, 16)
		if e1 != nil || e2 != nil || e3 != nil {
			break
		}
		days = append(days, float64(whole*100+tenth*10+hundredth)/100)
	}
	return days
}

// SetSwitch switches a device through the cloud (POST /v2/device/thing/status),
// the fallback when the device refuses the LAN command. multi is a device
// that reports "switches" (one entry per channel).
func (c *Cloud) SetSwitch(ctx context.Context, cfg CloudConfig, id string, on, multi bool) error {
	want := "off"
	if on {
		want = "on"
	}
	var params any = map[string]string{"switch": want}
	if multi {
		params = map[string]any{"switches": []map[string]any{{"switch": want, "outlet": 0}}}
	}
	body, _ := json.Marshal(map[string]any{"type": 1, "id": id, "params": params})
	_, err := c.call(ctx, cfg, http.MethodPost, "/v2/device/thing/status", nil, body, "Bearer "+cfg.AccessToken)
	return err
}
