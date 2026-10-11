// Command dreamhub is DreamHub's backend: a single static binary serving
// the /api/v1 REST API (and, later, the embedded web frontend — see
// ROADMAP.md, Foundation). See ARCHITECTURE.md for why a static Go binary
// over systemd is the right shape for this platform, and PRINCIPLES.md for
// the resource budget this is built to respect.
package main

import (
	"context"
	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	atmock "github.com/antonbier/DreamHub/backend/internal/autotimer/mock"
	"github.com/antonbier/DreamHub/backend/internal/dmi"
	"github.com/antonbier/DreamHub/backend/internal/fanart"
	fanartmock "github.com/antonbier/DreamHub/backend/internal/fanart/mock"
	"github.com/antonbier/DreamHub/backend/internal/omdb"
	omdbmock "github.com/antonbier/DreamHub/backend/internal/omdb/mock"
	"github.com/antonbier/DreamHub/backend/internal/plex"
	plexmock "github.com/antonbier/DreamHub/backend/internal/plex/mock"
	"github.com/antonbier/DreamHub/backend/internal/push"
	pushmock "github.com/antonbier/DreamHub/backend/internal/push/mock"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/tmdb"
	tmdbmock "github.com/antonbier/DreamHub/backend/internal/tmdb/mock"
	"github.com/antonbier/DreamHub/backend/internal/tvmaze"
	tvmazemock "github.com/antonbier/DreamHub/backend/internal/tvmaze/mock"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	aimock "github.com/antonbier/DreamHub/backend/internal/ai/mock"
	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/app"
	"github.com/antonbier/DreamHub/backend/internal/config"
	"github.com/antonbier/DreamHub/backend/internal/journald"
	jmock "github.com/antonbier/DreamHub/backend/internal/journald/mock"
	"github.com/antonbier/DreamHub/backend/internal/logging"
	"github.com/antonbier/DreamHub/backend/internal/scripts"
	scriptsmock "github.com/antonbier/DreamHub/backend/internal/scripts/mock"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	srmock "github.com/antonbier/DreamHub/backend/internal/serienrecorder/mock"
	"github.com/antonbier/DreamHub/backend/internal/systemd"
	sdmock "github.com/antonbier/DreamHub/backend/internal/systemd/mock"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
	"github.com/antonbier/DreamHub/backend/internal/webui"
)

func main() {
	level := logging.NewLevel(nil)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level.Leveler()}))
	if err := run(logger, level); err != nil {
		logger.Error("dreamhub stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, level *logging.Level) error {
	restrictFileModes()
	cfg := config.Load()

	var wc webcontrol.Client
	var sr serienrecorder.Client
	var aiClient ai.Client
	var at autotimer.Client
	var recFS recfs.FS
	var tmdbClient tmdb.Client
	var tvmazeClient tvmaze.Client
	var omdbClient omdb.Client
	var fanartClient fanart.Client
	var pushClient push.Client
	var plexClient plex.Client
	var streamBase string
	var dmiReader *dmi.Reader
	var logs journald.Reader
	var services systemd.Manager
	var scriptRunner scripts.Runner
	switch cfg.Mode {
	case config.ModeLive:
		c := webcontrol.NewHTTPClient(cfg.WebControlURL)
		c.AllowWrites = cfg.EnableDeviceWrites
		wc = c
		// SerienRecorder's API lives on the same device, same host/port,
		// just a different path (ARCHITECTURE.md, issue #10).
		sr = serienrecorder.NewHTTPClient(cfg.WebControlURL)
		at = autotimer.NewHTTPClient(cfg.WebControlURL)
		// The real file system. MediaHub only offers file operations when
		// the box's recording folders actually exist on it, i.e. when
		// DreamHub runs on the box (recfs.Available).
		recFS = recfs.OS{}
		// The AI provider is a separate, unrelated external service (not on
		// the device), but follows the same single mock/live switch as
		// everything else, per PRINCIPLES.md's "Mock-first development".
		// It still won't be called without a real API key configured in
		// Settings (connector.OpenAI's Check, and ai.OpenAIClient's own
		// ErrNotConfigured).
		aiClient = ai.NewOpenAIClient()
		tmdbClient = tmdb.NewHTTPClient()
		tvmazeClient = tvmaze.NewHTTPClient()
		omdbClient = omdb.NewHTTPClient()
		fanartClient = fanart.NewHTTPClient()
		pushClient = push.NewHTTPClient()
		plexClient = plex.NewHTTPClient(api.Version)
		// The box's media player reaches DreamHub on the same machine —
		// only when DreamHub runs on the box (its enigma2 config is here).
		if _, err := os.Stat("/etc/enigma2"); err == nil {
			streamBase = "http://127.0.0.1:" + listenPort(cfg.ListenAddr)
		}
		dmiReader = &dmi.Reader{}
		logs = journald.Exec{}
		services = systemd.Exec{}
		scriptRunner = scripts.Exec{}
		logger.Info("webcontrol client mode", "mode", "live", "url", cfg.WebControlURL, "deviceWrites", cfg.EnableDeviceWrites)
	default:
		// DREAMHUB_MOCK_DATE (YYYY-MM-DD) moves the fixture evening to a
		// real day, for working in the browser on today's evening, or on
		// a day the clocks change (mock.SetDay must run before mock.New).
		var dayShift time.Duration
		if s := os.Getenv("DREAMHUB_MOCK_DATE"); s != "" {
			if day, err := time.Parse("2006-01-02", s); err == nil {
				before := mock.Evening
				mock.SetDay(day)
				dayShift = mock.Evening.Sub(before)
			} else {
				logger.Warn("DREAMHUB_MOCK_DATE ignored, not YYYY-MM-DD", "value", s)
			}
		}
		m := mock.New()
		if dayShift != 0 {
			// The fixture evening moved to a real day, so "now" should
			// follow the wall clock too, not stay pinned at 21:10.
			m.Now = time.Now
		}
		// A real MPEG-TS file to "stream live" in mock mode (StreamHub);
		// none ships with the repository.
		m.StreamFile = os.Getenv("DREAMHUB_MOCK_STREAM")
		// …and the same file as a recording, to try playing one.
		if b, err := os.ReadFile(m.StreamFile); m.StreamFile != "" && err == nil {
			m.FS.Put(mock.Root+"20260923 2015 - ZDF HD - Stream sample.ts", string(b))
		}
		// DREAMHUB_MOCK_CHAOS makes the mock box behave like a busy real
		// one ("slow") or an unreliable one ("flaky"); see chaos.go for
		// why that catches bugs an instant mock never would.
		switch os.Getenv("DREAMHUB_MOCK_CHAOS") {
		case "slow":
			m.Chaos = mock.Slow()
		case "flaky":
			m.Chaos = mock.Flaky()
		}
		// DREAMHUB_MOCK_RECORDING=1 starts the mock box with a recording
		// already running, like the real reference box right now.
		if os.Getenv("DREAMHUB_MOCK_RECORDING") == "1" {
			m.RecordNow(time.Now())
		}
		wc = m
		recFS = m.FS
		// DREAMHUB_MOCK_SR=absent simulates a box without SerienRecorder
		// installed, for testing the "plugin not there" path.
		if os.Getenv("DREAMHUB_MOCK_SR") == "absent" {
			sr = srmock.Absent{}
		} else {
			sr = &srmock.Client{Shift: int64(dayShift.Seconds())}
		}
		at = atmock.New()
		aiClient = aimock.New()
		tmdbClient = tmdbmock.New()
		tvmazeClient = tvmazemock.New()
		omdbClient = omdbmock.New()
		fanartClient = fanartmock.New()
		pushClient = pushmock.New()
		plexClient = plexmock.New()
		// The mock box "plays" whatever it's given.
		streamBase = "http://127.0.0.1:" + listenPort(cfg.ListenAddr)
		logs = jmock.New()
		services = sdmock.New()
		scriptRunner = &scriptsmock.Runner{}
		logger.Info("webcontrol client mode", "mode", "mock")
	}

	a, err := app.New(context.Background(), app.Options{
		DataDir:      cfg.DataDir,
		WebControl:   wc,
		Series:       sr,
		AutoTimer:    at,
		TMDB:         tmdbClient,
		TVmaze:       tvmazeClient,
		OMDb:         omdbClient,
		Fanart:       fanartClient,
		Push:         pushClient,
		Plex:         plexClient,
		StreamBase:   streamBase,
		DMI:          dmiReader,
		RecordingsFS: recFS,
		// Same switch as WebControl writes; mock mode always writes.
		ReadOnlyFiles: cfg.Mode == config.ModeLive && !cfg.EnableDeviceWrites,
		AI:            aiClient,
		Logs:          logs,
		Systemd:       services,
		Scripts:       scriptRunner,
		Mock:          cfg.Mode == config.ModeMock,
		LookPath:      exec.LookPath,
		Logger:        logger,
		Level:         level,
		WebUI:         webui.Handler(),
	})
	if err != nil {
		return err
	}
	defer a.Store.Close()

	// The box login lives in settings, which only exist once the app is
	// built — so the live clients get it late, per request.
	if c, ok := wc.(*webcontrol.HTTPClient); ok {
		c.Credentials = a.BoxCredentials
	}
	if c, ok := sr.(*serienrecorder.HTTPClient); ok {
		c.Credentials = a.BoxCredentials
	}
	if c, ok := at.(*autotimer.HTTPClient); ok {
		c.Credentials = a.BoxCredentials
	}

	go a.Poller.Run(context.Background())
	for _, job := range a.Background {
		go job(context.Background())
	}

	logger.Info("dreamhub starting", "version", api.Version, "addr", cfg.ListenAddr, "dataDir", cfg.DataDir)
	return http.ListenAndServe(cfg.ListenAddr, a.Handler)
}

// listenPort is the port of a listen address (":8080", "0.0.0.0:8080").
func listenPort(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return port
	}
	return "8080"
}
