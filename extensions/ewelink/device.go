package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Device is what's known about one device right now.
type Device struct {
	ID    string
	Type  string // TXT "type", e.g. "plug"
	Addr  string // ip:port for commands
	Seq   string // TXT "seq", bumps on every state change
	State map[string]any
	Err   error // why State is missing (no key, wrong key)
	Seen  time.Time
}

// On reports the switch state; ok is false when it isn't known.
func (d Device) On() (on, ok bool) {
	if s, isStr := d.State["switch"].(string); isStr {
		return s == "on", true
	}
	if list, isList := d.State["switches"].([]any); isList && len(list) > 0 {
		if m, isMap := list[0].(map[string]any); isMap {
			if s, isStr := m["switch"].(string); isStr {
				return s == "on", true
			}
		}
	}
	return false, false
}

// Fingerprint changes whenever the device's state does.
func (d Device) Fingerprint() string {
	on, ok := d.On()
	return fmt.Sprintf("%s|%s|%t|%t", d.ID, d.Seq, ok, on)
}

// Devices keeps the latest announcement of every device, refreshed by
// browsing the LAN in the background so /widgets answers at once.
type Devices struct {
	cfg    *ConfigStore
	browse func(context.Context, time.Duration) ([]Announcement, error)
	client *http.Client

	mu   sync.Mutex
	list map[string]Device
}

func NewDevices(cfg *ConfigStore) *Devices {
	return &Devices{cfg: cfg, browse: Browse, client: &http.Client{Timeout: 5 * time.Second}, list: map[string]Device{}}
}

// Refresh browses once and updates the cache.
func (ds *Devices) Refresh(ctx context.Context) error {
	anns, err := ds.browse(ctx, 2*time.Second)
	if err != nil {
		return err
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for _, a := range anns {
		ds.list[a.TXT["id"]] = ds.fromAnnouncement(a)
	}
	return nil
}

func (ds *Devices) fromAnnouncement(a Announcement) Device {
	id := a.TXT["id"]
	d := Device{ID: id, Type: a.TXT["type"], Seq: a.TXT["seq"], Seen: time.Now()}
	port := a.Port
	if port == 0 {
		port = 8081
	}
	if a.IP != nil {
		d.Addr = net.JoinHostPort(a.IP.String(), strconv.Itoa(int(port)))
	}
	conf, _ := ds.cfg.Get().device(id)
	d.State, d.Err = DecryptTXT(a.TXT, conf.Key)
	return d
}

// Snapshot is every device seen, configured ones first.
func (ds *Devices) Snapshot() []Device {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	var configured, other []Device
	for _, d := range ds.list {
		if _, ok := ds.cfg.Get().device(d.ID); ok {
			configured = append(configured, d)
		} else {
			other = append(other, d)
		}
	}
	sortDevices(configured)
	sortDevices(other)
	return append(configured, other...)
}

func (ds *Devices) Get(id string) (Device, bool) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	d, ok := ds.list[id]
	return d, ok
}

// Switch sends the eWeLink LAN command and waits until the device
// announces the new state (or gives up after a few seconds).
func (ds *Devices) Switch(ctx context.Context, id string, on bool) error {
	d, ok := ds.Get(id)
	if !ok || d.Addr == "" {
		return errors.New("device not found on the network")
	}
	conf, _ := ds.cfg.Get().device(id)
	want := "off"
	if on {
		want = "on"
	}
	// A single-channel device takes /zeroconf/switch; one that reports
	// "switches" (the S60TPF does) takes /zeroconf/switches with the channel
	// listed — the single-channel call is refused with error 400.
	path := "/zeroconf/switch"
	var payload any = map[string]string{"switch": want}
	if _, multi := d.State["switches"]; multi {
		path = "/zeroconf/switches"
		payload = map[string]any{"switches": []map[string]any{{"switch": want, "outlet": 0}}}
	}
	plain, _ := json.Marshal(payload)
	iv, data, err := Encrypt(conf.Key, plain)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"sequence": strconv.FormatInt(time.Now().UnixMilli(), 10), "deviceid": id, "selfApikey": "123",
		"iv": iv, "encrypt": true, "data": data,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+d.Addr+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ds.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var res struct {
		Error int `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("device answered %d", resp.StatusCode)
	}
	if res.Error != 0 {
		return fmt.Errorf("device refused the command (error %d)", res.Error)
	}
	for i := 0; i < 8; i++ {
		_ = ds.Refresh(ctx)
		if now, ok := ds.Get(id); ok {
			if isOn, known := now.On(); known && isOn == on {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("the device accepted the command but didn't report the new state")
}

func sortDevices(l []Device) {
	for i := 1; i < len(l); i++ {
		for j := i; j > 0 && l[j].ID < l[j-1].ID; j-- {
			l[j], l[j-1] = l[j-1], l[j]
		}
	}
}
