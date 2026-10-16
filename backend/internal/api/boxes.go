package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/boxes"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// BoxParam is the query parameter that names the box a request is about
// (docs/MULTIDEVICE_PLAN.md). A query parameter, not a header, because
// EventSource, <img> and <video src> can't send headers. Omitted means
// the first box, so clients from before multi-box support keep working.
const BoxParam = "box"

// localOnlyPrefixes are the routes that administer the machine DreamHub
// runs on (services, packages, Docker, the terminal). They have no meaning
// for another box until that box runs DreamHub itself (docs/MULTIDEVICE_PLAN.md,
// "Basic" and "Full").
var localOnlyPrefixes = []string{"/api/v1/admin", "/api/v1/apps"}

// withBox resolves the request's box and puts it into the context:
// everything below reads it from there (internal/box). An unknown box is
// a 404; a box other than DreamHub's own machine is refused on the routes
// that administer that machine.
func withBox(d Deps, next http.Handler) http.Handler {
	if d.Boxes == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := httpx.User(r.Context())
		id := r.URL.Query().Get(BoxParam)
		if id == "" {
			// No box named: the first one this profile may use.
			for _, c := range d.Boxes.List() {
				if user.MayUseBox(c.ID) {
					id = c.ID
					break
				}
			}
			if id == "" {
				httpx.WriteError(w, r, http.StatusForbidden, "box_forbidden", "this profile may not use any box")
				return
			}
		}
		e, ok := d.Boxes.Get(id)
		if !ok {
			httpx.WriteError(w, r, http.StatusNotFound, "box_not_found", "no such box")
			return
		}
		if !user.MayUseBox(e.ID) {
			httpx.WriteError(w, r, http.StatusForbidden, "box_forbidden", "this profile may not use that box")
			return
		}
		ctx := box.With(r.Context(), e.ID)
		if !e.Local {
			ctx = box.WithRemote(ctx)
			for _, p := range localOnlyPrefixes {
				if r.URL.Path == p || strings.HasPrefix(r.URL.Path, p+"/") {
					httpx.WriteError(w, r, http.StatusConflict, "box_not_full", "this needs DreamHub running on that box")
					return
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// boxView is a box as clients see it. The address and login name are for
// the administrator only; the password never leaves the server.
type boxView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	Full   bool   `json:"full"`
	Online bool   `json:"online"`
	// Allowed: the caller's profile may use the box (an administrator also
	// sees the boxes they can't use, to manage them).
	Allowed bool `json:"allowed"`
	// Capabilities are measured for the boxes the caller may use and are
	// empty for one that doesn't answer (the list never waits for it).
	Capabilities []string `json:"capabilities"`
	URL          string   `json:"url,omitempty"`
	Username     string   `json:"username,omitempty"`
	HasPassword  bool     `json:"hasPassword,omitempty"`
}

func viewBox(d Deps, e boxes.Entry, admin bool, allowed bool) boxView {
	v := boxView{ID: e.ID, Name: e.Name, Color: e.Color, Full: e.Local, Online: d.Boxes.Online(e.ID), Allowed: allowed}
	if admin && !e.Local {
		v.URL, v.Username, v.HasPassword = e.URL, e.Username, e.Password != ""
	}
	return v
}

// handleListBoxes lists the boxes the caller may use; an administrator
// (who manages the list) sees all of them, with the connection details.
func handleListBoxes(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := perms(r).Has(perm.SettingsGlobal)
		out := []boxView{}
		def := "" // the box a request without `box` is about: the first the profile may use
		v, _ := d.Settings.Values(r.Context(), current(r))
		for _, e := range d.Boxes.List() {
			allowed := current(r).MayUseBox(e.ID)
			if allowed && def == "" {
				def = e.ID
			}
			if admin || allowed {
				bv := viewBox(d, e, admin, allowed)
				if allowed && bv.Online && v != nil {
					ctx := box.With(r.Context(), e.ID)
					bv.Capabilities = d.Boxes.Capabilities(ctx, d.Connectors.Reports(ctx, v, d.Settings.Version()))
				}
				if bv.Capabilities == nil {
					bv.Capabilities = []string{}
				}
				out = append(out, bv)
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"boxes": out, "default": def})
	}
}

// writeBoxError answers a box-management error; false when err is nil.
func writeBoxError(w http.ResponseWriter, r *http.Request, err error) bool {
	var ve *boxes.ValidationError
	switch {
	case err == nil:
		return false
	case errors.As(err, &ve):
		fields := map[string]httpx.FieldError{}
		for k, c := range ve.Fields {
			fields[k] = httpx.FieldError{Code: c}
		}
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "invalid box", fields)
	case errors.Is(err, boxes.ErrUnauthorized):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "box_unauthorized", "the box wants a login, or this one is wrong")
	case errors.Is(err, boxes.ErrUnreachable):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "box_unreachable", "the box does not answer at that address")
	case errors.Is(err, boxes.ErrUnknownBox), errors.Is(err, store.ErrBoxNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "box_not_found", "no such box")
	case errors.Is(err, boxes.ErrMainBox):
		httpx.WriteError(w, r, http.StatusConflict, "box_is_main", "the first box can't be removed")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not change the boxes")
	}
	return true
}

// handleTestBox tries an address and login without saving anything.
func handleTestBox(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in boxes.Input
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"url", "username", "password"}`)
			return
		}
		b, err := d.BoxManager.Candidate(in)
		if writeBoxError(w, r, err) {
			return
		}
		res, err := d.BoxManager.Test(r.Context(), b)
		if writeBoxError(w, r, err) {
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"deviceName": res.DeviceName, "tuners": res.Tuners})
	}
}

func handleAddBox(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in boxes.Input
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name", "color", "url", "username", "password"}`)
			return
		}
		e, err := d.BoxManager.Add(r.Context(), in)
		if writeBoxError(w, r, err) {
			return
		}
		logBoxChange(d, r, "box.add", "Added box "+e.Name)
		httpx.WriteJSON(w, http.StatusCreated, viewBox(d, e, true, true))
	}
}

func handleUpdateBox(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in boxes.Input
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name", "color", "url", "username", "password"}`)
			return
		}
		e, err := d.BoxManager.Update(r.Context(), r.PathValue("id"), in)
		if writeBoxError(w, r, err) {
			return
		}
		logBoxChange(d, r, "box.update", "Changed box "+e.Name)
		httpx.WriteJSON(w, http.StatusOK, viewBox(d, e, true, true))
	}
}

// handleRemoveBoxPlan previews removing a box: what DreamHub keeps for it
// is deleted with it, and that is said before anything happens.
func handleRemoveBoxPlan(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		e, ok := d.Boxes.Get(id)
		if !ok {
			writeBoxError(w, r, boxes.ErrUnknownBox)
			return
		}
		if id == d.Boxes.DefaultID() {
			writeBoxError(w, r, boxes.ErrMainBox)
			return
		}
		p := d.Plans.Create(r.Context(), current(r), plan.Draft{
			Action:      "box.remove",
			Changes:     []plan.Change{{Type: "box.remove", Before: viewBox(d, e, true, true)}},
			Summary:     "Remove box " + e.Name,
			Detail:      map[string]string{"box": id},
			Fingerprint: id,
			Current: func(ctx context.Context) (string, error) {
				if _, ok := d.Boxes.Get(id); !ok {
					return "", nil
				}
				return id, nil
			},
			Execute: func(ctx context.Context) error { return d.BoxManager.Remove(ctx, id) },
		})
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": p})
	}
}

func logBoxChange(d Deps, r *http.Request, action, summary string) {
	u := current(r)
	if err := d.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: u.ID, Action: action, Summary: summary}); err != nil {
		d.Logger.Error("recording a box change failed", "requestId", httpx.RequestID(r.Context()), "error", err)
	}
}

// boxStatus is one line of the dashboard's strip of other boxes.
type boxStatus struct {
	ID        string `json:"id"`
	Online    bool   `json:"online"`
	Standby   bool   `json:"standby,omitempty"`
	Service   string `json:"service,omitempty"`
	Recording bool   `json:"recording,omitempty"`
}

// overviewTimeout bounds how long the strip waits for a box.
const overviewTimeout = 3 * time.Second

// handleBoxesOverview says, for each box the caller may use, whether it
// answers, is in standby, what it shows and whether it records — what the
// dashboard's strip of the other boxes needs. A box that does not answer
// is reported offline at once and never waited for.
func handleBoxesOverview(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var list []boxes.Entry
		for _, e := range d.Boxes.List() {
			if current(r).MayUseBox(e.ID) {
				list = append(list, e)
			}
		}
		out := make([]boxStatus, len(list))
		var wg sync.WaitGroup
		for i, e := range list {
			out[i] = boxStatus{ID: e.ID, Online: d.Boxes.Online(e.ID)}
			if !out[i].Online {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(box.With(r.Context(), e.ID), overviewTimeout)
				defer cancel()
				st := &out[i]
				standby, err := e.WC.InStandby(ctx)
				if err != nil {
					st.Online = false
					return
				}
				st.Standby = standby
				if !standby {
					if cur, err := e.WC.Current(ctx); err == nil {
						st.Service = cur.ServiceName
					}
				}
				if timers, err := e.WC.TimerList(ctx); err == nil {
					for _, t := range timers {
						if t.State == webcontrol.TimerRunning && !t.Disabled {
							st.Recording = true
						}
					}
				}
			}()
		}
		wg.Wait()
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"boxes": out})
	}
}
