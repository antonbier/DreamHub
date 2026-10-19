package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config is the extension's own file, by default
// /var/lib/dreamhub-ext-ewelink/config.json (DREAMHUB_EWELINK_CONFIG
// overrides it). Device keys are secrets: the file is the extension
// user's own, mode 0600.
type Config struct {
	// DreamHub is where to register (default http://127.0.0.1:8765).
	DreamHub string `json:"dreamhub"`
	// Listen is this extension's own loopback address (default
	// 127.0.0.1:9101).
	Listen string `json:"listen"`
	// Lang picks the widget texts, "de" or "en" (default "de") — an
	// extension owns no DreamHub locale file, so it brings its own.
	Lang    string         `json:"lang"`
	Devices []DeviceConfig `json:"devices"`
	// Cloud is the owner's eWeLink developer application and its login:
	// used to fetch the device keys once and to read the consumption
	// history, never to switch (cloud.go).
	Cloud CloudConfig `json:"cloud"`
}

// DeviceConfig is one known device.
type DeviceConfig struct {
	// ID is the eWeLink device id (the mDNS TXT "id", e.g. 1002a17548).
	ID string `json:"id"`
	// Key is the devicekey from the owner's eWeLink account; without it
	// an encrypted device can be found but neither read nor switched.
	Key string `json:"key"`
	// Name is shown on the card (default: the id).
	Name string `json:"name"`
	// OffWarning is shown in the preview before switching off — e.g.
	// what's plugged in and must be shut down first.
	OffWarning string `json:"offWarning"`
}

func loadConfig(path string) (Config, error) {
	cfg := Config{DreamHub: "http://127.0.0.1:8765", Listen: "127.0.0.1:9101", Lang: "de"}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Lang != "en" {
		cfg.Lang = "de"
	}
	return cfg, nil
}

func (c Config) device(id string) (DeviceConfig, bool) {
	for _, d := range c.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return DeviceConfig{}, false
}

// ConfigStore is the live config: read on every use, changed from the
// settings form and written back to the file (mode 0600, atomically).
type ConfigStore struct {
	mu   sync.Mutex
	path string // "" keeps it in memory (tests)
	cfg  Config
}

func NewConfigStore(path string, cfg Config) *ConfigStore { return &ConfigStore{path: path, cfg: cfg} }

// Get returns a copy, safe to read while a save runs.
func (s *ConfigStore) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Devices = append([]DeviceConfig(nil), s.cfg.Devices...)
	return c
}

// Update applies fn to a copy and saves it; nothing changes if fn fails
// or the file can't be written.
func (s *ConfigStore) Update(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Devices = append([]DeviceConfig(nil), s.cfg.Devices...)
	if err := fn(&c); err != nil {
		return err
	}
	if s.path != "" {
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-*")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(b); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), s.path); err != nil { // CreateTemp makes it 0600
			return err
		}
	}
	s.cfg = c
	return nil
}
