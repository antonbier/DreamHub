package hubs

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Auto-start after boot (docs/APPSHUB_PLAN.md, App13). The choice is
// DreamHub's own (apps.autostartIds), not Docker's restart policy: the
// containers keep "on-failure, at most 5 retries", so an app that crashes is
// never restarted forever (the crash-loop guard stays exactly as it was),
// and the helper needs no new call. Once per boot, as soon as Docker
// answers, the apps on the list that are not running are started, helpers
// first. A restart of DreamHub alone, or an app somebody stopped, is not
// touched: the pass is remembered per boot.

func (h *Apps) bootID() string {
	if h.BootID != nil {
		return h.BootID()
	}
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// autostartPass starts what should be running; it reports false while Docker
// does not answer yet.
func (h *Apps) autostartPass(ctx context.Context, logger *slog.Logger) bool {
	boot := h.bootID()
	cfg := h.config(ctx)
	if h.Docker == nil || h.ReadOnly || boot == "" || cfg.BootID == boot {
		return true
	}
	containers, err := h.Docker.List(ctx)
	if err != nil {
		return false
	}
	for _, g := range groupApps(containers) {
		if !cfg.isAutostart(g.appID) {
			continue
		}
		order := g.all()
		for i := len(order) - 1; i >= 0; i-- { // helpers first
			c := order[i]
			if c.State == "running" || c.State == "restarting" || c.State == "paused" {
				continue
			}
			if err := h.Docker.Start(ctx, c.ID); err != nil && logger != nil {
				logger.Warn("auto-start failed", "app", g.appID, "error", err)
			}
		}
	}
	_ = h.saveSetting(ctx, KeyAppsBootID, boot)
	return true
}

// AutostartOnBoot runs until ctx is done or the pass has run: Docker
// comes up a little after DreamHub, so it asks again until it answers (for
// ten minutes at most).
func (h *Apps) AutostartOnBoot(ctx context.Context, logger *slog.Logger) {
	deadline := time.Now().Add(10 * time.Minute)
	for {
		if h.autostartPass(ctx, logger) || time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}
	}
}
