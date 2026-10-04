package api

import (
	"errors"
	"net/http"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/push"
)

// handlePushTest sends a test notification with the saved settings, so
// the user sees right away whether their phone gets it. Sends a message
// to an external service, but changes nothing — no plan needed.
func handlePushTest(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Push == nil {
			httpx.WriteError(w, r, http.StatusConflict, "push_not_configured", "push notifications aren't available")
			return
		}
		switch err := d.Push.Test(r.Context()); {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"sent": true})
		case errors.Is(err, push.ErrNotConfigured):
			httpx.WriteError(w, r, http.StatusConflict, "push_not_configured", "set an ntfy topic in Settings first")
		case errors.Is(err, push.ErrInvalidConfig):
			httpx.WriteError(w, r, http.StatusBadRequest, "push_invalid", "the ntfy server or topic is invalid")
		default:
			httpx.WriteError(w, r, http.StatusBadGateway, "push_failed", "the ntfy server couldn't be reached or refused the message")
		}
	}
}
