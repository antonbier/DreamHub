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
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/tmdb"
	tmdbmock "github.com/antonbier/DreamHub/backend/internal/tmdb/mock"
	"log/slog"
	"net/http"
	"os"
	"os/exec"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	aimock "github.com/antonbier/DreamHub/backend/internal/ai/mock"
	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/app"
	"github.com/antonbier/DreamHub/backend/internal/config"
	"github.com/antonbier/DreamHub/backend/internal/logging"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	srmock "github.com/antonbier/DreamHub/backend/internal/serienrecorder/mock"
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
		logger.Info("webcontrol client mode", "mode", "live", "url", cfg.WebControlURL, "deviceWrites", cfg.EnableDeviceWrites)
	default:
		m := mock.New()
		wc = m
		recFS = m.FS
		sr = srmock.New()
		at = atmock.New()
		aiClient = aimock.New()
		tmdbClient = tmdbmock.New()
		logger.Info("webcontrol client mode", "mode", "mock")
	}

	a, err := app.New(context.Background(), app.Options{
		DataDir:      cfg.DataDir,
		WebControl:   wc,
		Series:       sr,
		AutoTimer:    at,
		TMDB:         tmdbClient,
		RecordingsFS: recFS,
		// Same switch as WebControl writes; mock mode always writes.
		ReadOnlyFiles: cfg.Mode == config.ModeLive && !cfg.EnableDeviceWrites,
		AI:            aiClient,
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

	logger.Info("dreamhub starting", "version", api.Version, "addr", cfg.ListenAddr, "dataDir", cfg.DataDir)
	return http.ListenAndServe(cfg.ListenAddr, a.Handler)
}
