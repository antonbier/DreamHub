// Package config loads DreamHub's runtime configuration from the environment.
package config

import (
	"os"
)

// Mode selects which WebControl backend the webcontrol client talks to.
type Mode string

const (
	// ModeLive talks to a real device's WebControl API.
	ModeLive Mode = "live"
	// ModeMock talks to the built-in mock WebControl server, matching the
	// schemas documented in ARCHITECTURE.md. See PRINCIPLES.md, "Mock-first
	// development" — this is what makes DreamHub buildable and testable
	// without a reachable physical device.
	ModeMock Mode = "mock"
)

// Config holds everything read from the environment at startup.
type Config struct {
	// Mode is ModeLive or ModeMock. Defaults to ModeMock so the backend is
	// runnable out of the box without a device.
	Mode Mode

	// ListenAddr is the address the API server binds to.
	ListenAddr string

	// WebControlURL is the base URL of the WebControl API to call in
	// ModeLive. Ignored in ModeMock.
	WebControlURL string

	// DataDir is where DreamHub's own persistent state (SQLite database)
	// lives. On a real device this is under /data/dreamhub — see
	// PRINCIPLES.md, "Data storage: one SQLite file, on /data".
	DataDir string

	// AuthEnabled toggles Basic Auth / API token enforcement for the web
	// UI and API. Off by default — enabled from the Settings UI once it
	// exists, per PRINCIPLES.md's auth section.
	AuthEnabled bool
}

// Load reads configuration from the environment, applying defaults for
// anything not set. Nothing here requires network or filesystem access
// beyond the data directory, so it's safe to call in tests.
func Load() Config {
	cfg := Config{
		Mode:          Mode(getEnv("DREAMHUB_MODE", string(ModeMock))),
		ListenAddr:    getEnv("DREAMHUB_LISTEN_ADDR", ":8080"),
		WebControlURL: getEnv("DREAMHUB_WEBCONTROL_URL", "http://127.0.0.1"),
		DataDir:       getEnv("DREAMHUB_DATA_DIR", "/data/dreamhub"),
		AuthEnabled:   getEnv("DREAMHUB_AUTH_ENABLED", "false") == "true",
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
