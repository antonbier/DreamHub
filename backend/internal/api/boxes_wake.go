package api

import (
	"context"
	"net/http"

	"github.com/antonbier/DreamHub/backend/internal/boxes"
	"github.com/antonbier/DreamHub/backend/internal/extension"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
)

// handleWakeBox switches a box that is off on (docs/MULTIDEVICE_PLAN.md,
// M9). With Wake-on-LAN the packet goes out at once ({"sent": true}) —
// harmless if the box is already on. With an extension the answer is a
// plan carrying the extension's own sentence ("the socket is switched
// on"), because a socket may toggle: the person sees what it will do
// before it does it.
func handleWakeBox(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		e, ok := d.Boxes.Get(id)
		if !ok {
			writeBoxError(w, r, boxes.ErrUnknownBox)
			return
		}
		if !current(r).MayUseBox(id) {
			httpx.WriteError(w, r, http.StatusForbidden, "box_forbidden", "this profile may not use that box")
			return
		}
		switch e.WakeMethod {
		case boxes.WakeWOL:
			if err := d.BoxManager.Wake(r.Context(), id); err != nil {
				writeBoxError(w, r, err)
				return
			}
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"sent": true})
		case boxes.WakeExtension:
			wakePlan(d, w, r, e)
		default:
			writeBoxError(w, r, boxes.ErrNoWake)
		}
	}
}

func wakePlan(d Deps, w http.ResponseWriter, r *http.Request, e boxes.Entry) {
	var client extension.Client
	if d.Extensions != nil {
		if x, found := d.Extensions.Get(e.WakeExt); found {
			client = x.Client
		}
	}
	if client == nil {
		httpx.WriteError(w, r, http.StatusConflict, "wake_extension_missing", "the extension that switches this box on is not running")
		return
	}
	action, id := e.WakeAction, e.ID
	preview, err := client.PlanAction(r.Context(), action)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "extension_unavailable", "could not reach the extension")
		return
	}
	fp := "wake|" + id + "|" + action + "|" + preview.Fingerprint
	p := d.Plans.Create(r.Context(), current(r), plan.Draft{
		Action:      "box.wake",
		Warnings:    preview.Warnings,
		Summary:     preview.Summary,
		Detail:      map[string]string{"box": id, "extension": e.WakeExt, "action": action},
		Fingerprint: fp,
		Current: func(ctx context.Context) (string, error) {
			pv, err := client.PlanAction(ctx, action)
			if err != nil {
				return "", err
			}
			return "wake|" + id + "|" + action + "|" + pv.Fingerprint, nil
		},
		Execute: func(ctx context.Context) error {
			if err := client.Apply(ctx, action, preview.Fingerprint); err != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "extension_apply_failed", Message: "the extension could not apply this", Err: err}
			}
			if d.BoxManager != nil && d.BoxManager.OnWake != nil {
				d.BoxManager.OnWake(id)
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": p})
}
