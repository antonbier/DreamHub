package hubs

import (
	"context"
	"errors"
	"net/http"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Disabling and enabling a timer (decision D5, docs/V1_PLAN.md). The box's
// own /web/timerchange has a bug in its conflict path that leaves a timer
// half-changed (ARCHITECTURE.md), so DreamHub doesn't use it: a timer is
// taken out with /web/timerdelete and put back with /web/timeradd —
// unchanged except for "disabled", every other field (folder, tags, after
// event, repeat, EPG link) carried over — the two write endpoints
// confirmed live. All or nothing: if putting it back fails (the box finds
// a conflict on enabling, say), the timer as it was is put back instead.
//
// A timer that is recording right now is refused: taking it out would
// abort the recording (the box aborts a running timer on delete). Enabling
// shows the same conflict estimate as creating a timer; the box runs its
// own exact check when the timer goes back in.

// CodeTimerToggleRunning: a running timer can't be disabled this way.
const CodeTimerToggleRunning = "timer_toggle_running"

// CodeTimerRestored: putting the timer back changed failed, and the
// original was restored — nothing changed in the end.
const CodeTimerRestored = "timer_toggle_restored"

type toggleRequest struct {
	ServiceReference string `json:"serviceReference"`
	Begin            int64  `json:"begin"`
	End              int64  `json:"end"`
	// Enabled is what the timer should become.
	Enabled bool `json:"enabled"`
}

func (h *Timer) planToggle(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	var req toggleRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || req.ServiceReference == "" || req.Begin <= 0 || req.End <= 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"serviceReference", "begin", "end", "enabled"} as listed by GET /api/v1/timers`)
		return
	}
	ctx := r.Context()
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	ref := webcontrol.TimerRef{ServiceReference: req.ServiceReference, Begin: req.Begin, End: req.End}
	t, ok := findTimer(timers, ref)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, CodeTimerNotFound, "no timer with this service reference, begin and end")
		return
	}
	if !h.mayChange(ctx, user, t) {
		httpx.WriteError(w, r, http.StatusForbidden, CodeTimerNotYours, "this profile may only change timers it made")
		return
	}
	if t.Disabled == !req.Enabled {
		httpx.WriteError(w, r, http.StatusConflict, "timer_unchanged", "the timer is already in that state")
		return
	}
	if t.Active() {
		httpx.WriteError(w, r, http.StatusConflict, CodeTimerToggleRunning, "the timer is recording right now; disabling it would stop the recording")
		return
	}
	if req.Enabled && t.End <= h.Now().Unix() {
		httpx.WriteError(w, r, http.StatusConflict, "timer_in_past", "the timer's time has passed; it can't be enabled again")
		return
	}

	var warnings []status.Problem
	if req.Enabled {
		info, _ := h.WC.DeviceInfo(ctx)
		if clash := conflicts(timers, info, t.ServiceReference, t.Begin, t.End); len(clash) > 0 {
			with := make([]map[string]any, 0, len(clash))
			for _, c := range clash {
				with = append(with, map[string]any{"name": c.Name, "serviceName": c.ServiceName, "begin": c.Begin, "end": c.End})
			}
			warnings = append(warnings, status.Problem{Code: CodeTimerConflict, Params: map[string]any{"with": with, "tuners": info.Tuners(), "estimate": true}})
		}
	}

	original := t.AsNewTimer()
	changed := original
	changed.Disabled = !req.Enabled
	action, summary := "timer.disable", "Disabled timer: "
	if req.Enabled {
		action, summary = "timer.enable", "Enabled timer: "
	}
	after := t
	after.Disabled = !req.Enabled
	pl := h.Plans.Create(user, plan.Draft{
		Action:      action,
		Changes:     []plan.Change{{Type: action, Before: t, After: after}},
		Warnings:    warnings,
		Summary:     summary + t.Name + " (" + t.ServiceName + ")",
		Detail:      t,
		Fingerprint: fingerprint(timers),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.WC.TimerList(ctx)
			if err != nil {
				return "", err
			}
			return fingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			if err := h.WC.DeleteTimer(ctx, ref); err != nil {
				return boxFailure(err)
			}
			if err := h.WC.AddTimer(ctx, changed); err == nil {
				if now, err := h.WC.TimerList(ctx); err == nil {
					if got, ok := findTimer(now, ref); ok && got.Disabled == changed.Disabled {
						return nil
					}
				}
			} else if errors.Is(err, webcontrol.ErrWritesDisabled) {
				return err
			}
			// Put the timer back as it was. Remove a half-added changed one
			// first, so the original's begin is free again.
			_ = h.WC.DeleteTimer(ctx, ref)
			if err := h.WC.AddTimer(ctx, original); err != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: CodeTimerNotCreated, Message: "the timer couldn't be changed or put back; recreate it", Details: map[string]any{"timer": original}, Err: err}
			}
			return &plan.Failure{Status: http.StatusConflict, Code: CodeTimerRestored, Message: "the box refused the changed timer; the timer was put back unchanged"}
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}
