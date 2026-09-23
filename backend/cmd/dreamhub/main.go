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
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("dreamhub stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := config.Load()

	var wc webcontrol.Client
	switch cfg.Mode {
	case config.ModeLive:
		wc = webcontrol.NewHTTPClient(cfg.WebControlURL)
		logger.Info("webcontrol client mode", "mode", "live", "url", cfg.WebControlURL)
	default:
		wc = mock.Client{}
		logger.Info("webcontrol client mode", "mode", "mock")
	}

	a, err := app.New(context.Background(), app.Options{
		DataDir:    cfg.DataDir,
		WebControl: wc,
		Mock:       cfg.Mode == config.ModeMock,
		LookPath:   exec.LookPath,
		Logger:     logger,
	})
	if err != nil {
		return err
	}
	defer a.Store.Close()

	logger.Info("dreamhub starting", "version", api.Version, "addr", cfg.ListenAddr, "dataDir", cfg.DataDir)
	return http.ListenAndServe(cfg.ListenAddr, a.Handler)
}
