// Package api implements DreamHub's REST API. Per PRINCIPLES.md's
// "API-first" principle, every route lives under /api/v1 and is the same
// API any future client (the web UI, a mobile app) would call — no
// web-UI-only backend logic.
package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Version is DreamHub's own version, following SemVer per PRINCIPLES.md.
// Reported by /api/v1/health so the Enigma2 plugin can detect a mismatch.
const Version = "0.1.0-dev"

// maxBodyBytes caps request bodies; settings updates are tiny.
const maxBodyBytes = 64 << 10

// Deps are the router's dependencies.
type Deps struct {
	WebControl webcontrol.Client
	Verifier   auth.Verifier
	Settings   *settings.Service
	// User is the current user. v1 has exactly one user (an admin); once
	// profiles exist this comes from the authenticated request instead.
	User   store.User
	Logger *slog.Logger
	Now    func() time.Time
}

// NewRouter builds the full /api/v1 handler tree.
func NewRouter(d Deps) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/deviceinfo", handleDeviceInfo(d))
	protected.HandleFunc("GET /api/v1/movies", handleMovies(d))
	if d.Settings != nil {
		protected.HandleFunc("GET /api/v1/settings", handleGetSettings(d))
		protected.HandleFunc("PATCH /api/v1/settings", handlePatchSettings(d))
	}

	mux := http.NewServeMux()
	// Public on purpose: the Enigma2 plugin checks the backend version before
	// it has any credentials, and must keep working when auth is enabled.
	mux.HandleFunc("GET /api/v1/health", handleHealth(d))
	mux.Handle("/api/v1/", auth.Middleware(d.Verifier, protected))

	return withLogging(d.Logger, mux)
}

func handleHealth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": Version,
			// Server time in UTC, so clients can warn when the box clock is off
			// (CONCEPT.md, "Time zones").
			"time": d.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleDeviceInfo(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, err := d.WebControl.DeviceInfo(r.Context())
		if err != nil {
			d.Logger.Error("deviceinfo failed", "error", err)
			writeError(w, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
			return
		}
		writeJSON(w, http.StatusOK, info)
	}
}

func handleMovies(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		movies, err := d.WebControl.MovieList(r.Context())
		if err != nil {
			d.Logger.Error("movielist failed", "error", err)
			writeError(w, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
			return
		}
		writeJSON(w, http.StatusOK, movies)
	}
}

func handleGetSettings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := d.Settings.View(r.Context(), d.User)
		if err != nil {
			d.Logger.Error("settings view failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "could not load settings")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sections": view})
	}
}

func handlePatchSettings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Values map[string]json.RawMessage `json:"values"`
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil || body.Values == nil {
			writeError(w, http.StatusBadRequest, "invalid_body", `expected {"values": {"<key>": <value>, ...}}`)
			return
		}
		if err := d.Settings.Update(r.Context(), d.User, body.Values); err != nil {
			if verr, ok := settings.IsValidation(err); ok {
				writeErrorDetails(w, http.StatusBadRequest, "validation_failed", "one or more settings are invalid; nothing was saved", verr.Fields)
				return
			}
			d.Logger.Error("settings update failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "could not save settings")
			return
		}
		view, err := d.Settings.View(r.Context(), d.User)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "saved, but could not reload settings")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sections": view})
	}
}

func withLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
