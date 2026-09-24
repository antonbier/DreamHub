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
	// lives. On a real device this is /data/dreamhub — see PRINCIPLES.md,
	// "Data storage: one SQLite file, on /data". In mock mode it defaults to
	// a local directory so development works on any machine.
	DataDir string

	// EnableDeviceWrites allows write operations against a real box (e.g.
	// creating timers). On by default since the timer write endpoints were
	// verified live on the reference device (ARCHITECTURE.md, "Timer write
	// endpoints"); DREAMHUB_ENABLE_DEVICE_WRITES=0 switches them off, e.g.
	// for a read-only trial run. Every write still goes through
	// preview-and-confirm. Has no effect in mock mode, where writes always
	// work.
	EnableDeviceWrites bool
}

// Load reads configuration from the environment, applying defaults for
// anything not set. Auth is not configured here: it lives in the settings
// store and is changed through the settings API/UI.
func Load() Config {
	mode := Mode(getEnv("DREAMHUB_MODE", string(ModeMock)))
	defaultDataDir := "/data/dreamhub"
	if mode == ModeMock {
		defaultDataDir = "./.dreamhub-data"
	}
	return Config{
		Mode:          mode,
		ListenAddr:    getEnv("DREAMHUB_LISTEN_ADDR", ":8080"),
		WebControlURL: getEnv("DREAMHUB_WEBCONTROL_URL", "http://127.0.0.1"),
		DataDir:       getEnv("DREAMHUB_DATA_DIR", defaultDataDir),

		EnableDeviceWrites: getEnv("DREAMHUB_ENABLE_DEVICE_WRITES", "1") != "0",
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
