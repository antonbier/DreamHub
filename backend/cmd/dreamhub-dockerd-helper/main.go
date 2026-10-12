// Command dreamhub-dockerd-helper is the only process that ever holds
// Docker's own socket (docs/APPSHUB_PLAN.md, App2 — "Docker access is
// isolated from DreamHub's own backend process"). It is deliberately
// small and does one thing: serve DreamHub's main backend a whitelisted,
// loopback-only subset of the Docker API (internal/dockerd.Server),
// scoped to containers carrying the dreamhub.app label.
//
// DreamHub's own backend never runs as this process and never holds the
// Docker socket itself — it only ever calls this one, over loopback, via
// internal/dockerd.HTTPClient. A bug in the much larger main backend can
// therefore not turn into Docker-socket-equivalent (root-equivalent)
// access to the box.
//
// Off by default: not started by the main package's postinst unless an
// admin turns Docker apps on in AppsHub (App4). Not wired into
// build-deb.sh/packaging yet — this command exists so App2 can be built
// and tested; shipping it as part of DreamHub's own .deb, with its own
// systemd unit and dedicated, docker-group-only user, is still open (see
// docs/APPSHUB_PLAN.md, "Open questions").
package main

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/dockerd/mock"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("dreamhub-dockerd-helper stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	// 127.0.0.1 only, never 0.0.0.0 — this helper has no auth of its own
	// (DreamHub's main backend is the only caller, and only it is meant
	// to reach this port); the systemd unit's own hardening (once built)
	// enforces the same restriction a layer deeper
	// (RestrictAddressFamilies / IPAddressAllow=localhost, the same
	// directives packaging/extension-template already uses).
	addr := envOr("DREAMHUB_DOCKERD_HELPER_ADDR", "127.0.0.1:8090")
	sockPath := envOr("DREAMHUB_DOCKER_SOCKET", "/var/run/docker.sock")

	var client dockerd.Client
	if os.Getenv("DREAMHUB_MOCK") == "1" {
		client = mock.New()
		logger.Info("dockerd-helper starting", "mode", "mock", "addr", addr)
	} else {
		client = dockerd.Exec{SocketPath: sockPath}
		logger.Info("dockerd-helper starting", "mode", "live", "addr", addr, "socket", sockPath)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           dockerd.Server{Client: client}.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.Serve(listener)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
