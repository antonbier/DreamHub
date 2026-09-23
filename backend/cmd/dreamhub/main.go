// Command dreamhub is DreamHub's backend: a single static binary serving
// the /api/v1 REST API (and, later, the embedded web frontend — see
// ROADMAP.md, Foundation). See ARCHITECTURE.md for why a static Go binary over systemd
// is the right shape for this platform, and PRINCIPLES.md for the
// resource budget this is built to respect.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/config"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

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

	verifier := auth.Credentials{Enabled: cfg.AuthEnabled}

	handler := api.NewRouter(wc, verifier, logger)

	logger.Info("dreamhub starting", "version", api.Version, "addr", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, handler); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
