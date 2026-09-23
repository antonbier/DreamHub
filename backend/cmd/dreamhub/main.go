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

	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/config"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
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
	ctx := context.Background()
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

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	user, err := st.EnsureDefaultUser(ctx)
	if err != nil {
		return err
	}

	reg := settings.NewRegistry()
	for _, sec := range settings.CoreSections() {
		if err := reg.Register(sec); err != nil {
			return err
		}
	}
	svc, err := settings.NewService(ctx, st, reg)
	if err != nil {
		return err
	}

	handler := api.NewRouter(api.Deps{
		WebControl: wc,
		Verifier:   auth.NewSettingsVerifier(svc),
		Settings:   svc,
		User:       user,
		Logger:     logger,
	})

	logger.Info("dreamhub starting", "version", api.Version, "addr", cfg.ListenAddr, "dataDir", cfg.DataDir)
	return http.ListenAndServe(cfg.ListenAddr, handler)
}
