// Package api implements DreamHub's REST API. Per PRINCIPLES.md's
// "API-first" principle, every route lives under /api/v1 and is the same
// API any future client (the web UI, a mobile app) would call — no
// web-UI-only backend logic.
package api

import (
	"log/slog"
	"net/http"

	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Version is DreamHub's own version, following SemVer per PRINCIPLES.md.
// Reported by /api/v1/health so the Enigma2 plugin can detect a mismatch
// with what it expects (see PRINCIPLES.md, "Plugin/backend version
// compatibility check").
const Version = "0.1.0-dev"

// NewRouter builds the full /api/v1 handler tree.
func NewRouter(wc webcontrol.Client, verifier auth.Verifier, logger *slog.Logger) http.Handler {
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/deviceinfo", handleDeviceInfo(wc, logger))
	protected.HandleFunc("GET /api/v1/movies", handleMovies(wc, logger))

	mux := http.NewServeMux()
	// Public on purpose: the Enigma2 plugin checks the backend version before
	// it has any credentials, and must keep working when auth is enabled.
	mux.HandleFunc("GET /api/v1/health", handleHealth)
	mux.Handle("/api/v1/", auth.Middleware(verifier, protected))

	return withLogging(logger, mux)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": Version,
	})
}

func handleDeviceInfo(wc webcontrol.Client, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, err := wc.DeviceInfo(r.Context())
		if err != nil {
			logger.Error("deviceinfo failed", "error", err)
			writeError(w, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
			return
		}
		writeJSON(w, http.StatusOK, info)
	}
}

func handleMovies(wc webcontrol.Client, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		movies, err := wc.MovieList(r.Context())
		if err != nil {
			logger.Error("movielist failed", "error", err)
			writeError(w, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
			return
		}
		writeJSON(w, http.StatusOK, movies)
	}
}

func withLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
