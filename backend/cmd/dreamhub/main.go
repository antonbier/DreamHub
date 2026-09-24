// Command dreamhub is DreamHub's backend: a single static binary serving
// the /api/v1 REST API (and, later, the embedded web frontend — see
// ROADMAP.md, Foundation). See ARCHITECTURE.md for why a static Go binary
// over systemd is the right shape for this platform, and PRINCIPLES.md for
// the resource budget this is built to respect.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/exec"

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
	cfg := config.Load()

	var wc webcontrol.Client
	var sr serienrecorder.Client
	switch cfg.Mode {
	case config.ModeLive:
		c := webcontrol.NewHTTPClient(cfg.WebControlURL)
		c.AllowWrites = cfg.EnableDeviceWrites
		wc = c
		// SerienRecorder's API lives on the same device, same host/port,
		// just a different path (ARCHITECTURE.md, issue #10).
		sr = serienrecorder.NewHTTPClient(cfg.WebControlURL)
		logger.Info("webcontrol client mode", "mode", "live", "url", cfg.WebControlURL, "deviceWrites", cfg.EnableDeviceWrites)
	default:
		wc = mock.New()
		sr = srmock.New()
		logger.Info("webcontrol client mode", "mode", "mock")
	}

	a, err := app.New(context.Background(), app.Options{
		DataDir:    cfg.DataDir,
		WebControl: wc,
		Series:     sr,
		Mock:       cfg.Mode == config.ModeMock,
		LookPath:   exec.LookPath,
		Logger:     logger,
		Level:      level,
		WebUI:      webui.Handler(),
	})
	if err != nil {
		return err
	}
	defer a.Store.Close()

	logger.Info("dreamhub starting", "version", api.Version, "addr", cfg.ListenAddr, "dataDir", cfg.DataDir)
	return http.ListenAndServe(cfg.ListenAddr, a.Handler)
}
