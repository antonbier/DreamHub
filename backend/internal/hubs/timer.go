// Package hubs contains the behavior of the individual hubs (their API
// routes). Their descriptors — navigation, requirements, status — live in
// internal/hub. Each hub mounts its own routes under /api/v1; the core only
// guards them (auth, enable switch).
package hubs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/tuner"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Timer is TimerHub: listing timers, creating and deleting them via
// preview-and-confirm, and a read-only view of the optional AutoTimer
// plugin.
type Timer struct {
	WC    webcontrol.Client
	Plans *plan.Service
	Now   func() time.Time
	// AT is the AutoTimer plugin's API; nil or "not installed" just hides
	// that section (the plugin is optional, docs/V1_PLAN.md rule 1).
	AT autotimer.Client
	// Owners records which profile made a timer through DreamHub, so a
	// profile without perm.TimersManage can delete or switch off its own
	// timers but no one else's (docs/PROFILES_PLAN.md). nil: nobody owns
	// anything.
	Owners TimerOwners
}

// TimerOwners is where timer ownership is kept (internal/store).
type TimerOwners interface {
	SetTimerOwner(ctx context.Context, serviceRef string, begin, userID int64) error
	TimerOwner(ctx context.Context, serviceRef string, begin int64) (int64, error)
}

// Perms are TimerHub's route permissions (internal/perm).
func (h *Timer) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"POST /api/v1/timers/plan":        perm.TimersPlan,
		"POST /api/v1/timers/delete/plan": perm.TimersPlan, // and mayChange
		"POST /api/v1/timers/toggle/plan": perm.TimersPlan, // and mayChange
	}
}

// mayChange reports whether user may delete or switch off t: anyone with
// perm.TimersManage, else only the profile that made it through DreamHub.
func (h *Timer) mayChange(ctx context.Context, user store.User, t webcontrol.Timer) bool {
	if perm.For(user).Has(perm.TimersManage) {
		return true
	}
	if h.Owners == nil {
		return false
	}
	owner, err := h.Owners.TimerOwner(ctx, t.ServiceReference, t.Begin)
	return err == nil && owner == user.ID
}

// Mount registers TimerHub's routes.
func (h *Timer) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/timers", h.list)
	mux.HandleFunc("POST /api/v1/timers/plan", h.planCreate)
	mux.HandleFunc("POST /api/v1/timers/delete/plan", h.planDelete)
	mux.HandleFunc("POST /api/v1/timers/toggle/plan", h.planToggle)
	mux.HandleFunc("GET /api/v1/timers/autotimers", h.autoTimers)
	mux.HandleFunc("GET /api/v1/timers/autotimers/simulate", h.autoTimerSimulate)
}

// AutoTimerEditorPath is the AutoTimer plugin's own web GUI on the box
// (WebControl's port 80), linked rather than rebuilt — decision D3.
const AutoTimerEditorPath = "/autotimereditor/"

// Margin defaults and limits, in minutes (the mockup's "5 / 10 min").
const (
	DefaultMarginBefore = 5
	DefaultMarginAfter  = 10
	MaxMargin           = 60
)

// Timer problem codes (i18n keys).
const (
	CodeTimerConflict  = "timer.conflict"
	CodeTimerDuplicate = "timer.duplicate"
	// CodeTimerDeleteRunning warns that deleting stops a recording in
	// progress — the box aborts a running timer on delete (ARCHITECTURE.md).
	CodeTimerDeleteRunning = "timer.deleteRunning"
)

// Apply failure codes (plan.Failure), i18n keys under "error".
const (
	// The box refused the change; details carry its own (translated) text.
	CodeBoxRejected = "box_rejected"
	// The box said yes, but the timer list says otherwise — typically a
	// timer on the same channel with the same start already existed, which
	// the box drops silently while reporting success.
	CodeTimerNotCreated = "timer_not_created"
	CodeTimerNotDeleted = "timer_not_deleted"
	CodeTimerNotFound   = "timer_not_found"
	CodeTimerNotYours   = "timer_not_yours" // a viewer may only delete or switch off timers it made
)

// Search implements hub.SearchProvider: a case-insensitive substring match
// over each timer's name and channel — TimerHub is the hub contract's first
// real search provider (ROADMAP.md).
func (h *Timer) Search(ctx context.Context, query string) ([]hub.SearchResult, error) {
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []hub.SearchResult
	for _, t := range timers {
		if strings.Contains(strings.ToLower(t.Name), q) || strings.Contains(strings.ToLower(t.ServiceName), q) {
			out = append(out, hub.SearchResult{Title: t.Name, Subtitle: t.ServiceName, Path: "/timers"})
		}
	}
	return out, nil
}

// Timer sources, derived from the tags the box reports.
const (
	SourceSerienRecorder = "serienrecorder"
	SourceAutoTimer      = "autotimer"
)

// TimerView is a timer as TimerHub lists it: the box's fields plus what
// DreamHub derives — who created it, and whether it likely clashes with
// another timer on this box's tuners (an estimate, like the plan warning).
type TimerView struct {
	webcontrol.Timer
	Source   string `json:"source,omitempty"`
	Conflict bool   `json:"conflict"`
	// MayChange: the current profile may delete or switch off this timer
	// (set by the list only).
	MayChange bool `json:"mayChange"`
}

// timerSource reads the creator from the tags: SerienRecorder always tags
// its timers "SerienRecorder" (live fixture); AutoTimer tags them
// "AutoTimer" when its add_autotimer_to_tags option is on (plugin source).
// Anything else is unknown — not necessarily manual.
func timerSource(tags string) string {
	for _, tag := range strings.Fields(tags) {
		switch tag {
		case "SerienRecorder":
			return SourceSerienRecorder
		case "AutoTimer":
			return SourceAutoTimer
		}
	}
	return ""
}

// timerViews marks every pending timer that likely conflicts with the
// others (internal/tuner).
func timerViews(timers []webcontrol.Timer, info *webcontrol.DeviceInfo) []TimerView {
	clash := tuner.TimerConflicts(timers, info)
	out := make([]TimerView, len(timers))
	for i, t := range timers {
		out[i] = TimerView{Timer: t, Source: timerSource(t.Tags), Conflict: clash[i]}
	}
	return out
}

func (h *Timer) list(w http.ResponseWriter, r *http.Request) {
	timers, err := h.WC.TimerList(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	info, _ := h.WC.DeviceInfo(r.Context())
	views := timerViews(timers, info)
	user, _ := httpx.User(r.Context())
	for i := range views {
		views[i].MayChange = h.mayChange(r.Context(), user, views[i].Timer)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"timers": views, "tuners": info.Tuners()})
}

// autoTimers lists the AutoTimer plugin's configuration. A box without the
// plugin is a normal answer ({"installed": false}), not an error.
func (h *Timer) autoTimers(w http.ResponseWriter, r *http.Request) {
	if h.AT == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"installed": false})
		return
	}
	list, err := h.AT.List(r.Context())
	switch {
	case errors.Is(err, autotimer.ErrNotInstalled):
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"installed": false})
	case errors.Is(err, autotimer.ErrUnauthorized):
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unauthorized", "the box asks for credentials; set them in Settings → WebControl")
	case err != nil:
		httpx.WriteError(w, r, http.StatusBadGateway, "autotimer_unavailable", "could not read the AutoTimer configuration")
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"installed": true, "autoTimers": list, "editorPath": AutoTimerEditorPath})
	}
}

// autoTimerSimulate is the plugin's own dry run: the timers the AutoTimers
// would create now. Read-only on the box (parseEPG(simulateOnly=True)), so
// no plan is needed.
func (h *Timer) autoTimerSimulate(w http.ResponseWriter, r *http.Request) {
	if h.AT == nil {
		httpx.WriteError(w, r, http.StatusNotFound, "autotimer_not_installed", "the AutoTimer plugin isn't installed on the box")
		return
	}
	sim, err := h.AT.Simulate(r.Context())
	switch {
	case errors.Is(err, autotimer.ErrNotInstalled):
		httpx.WriteError(w, r, http.StatusNotFound, "autotimer_not_installed", "the AutoTimer plugin isn't installed on the box")
	case err != nil:
		httpx.WriteError(w, r, http.StatusBadGateway, "autotimer_unavailable", "could not run the AutoTimer dry run")
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"timers": sim})
	}
}

type createRequest struct {
	ServiceReference string `json:"serviceReference"`
	ServiceName      string `json:"serviceName"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	// Begin and End are the broadcast times (Unix seconds, UTC); margins are
	// added on top.
	Begin        int64 `json:"begin"`
	End          int64 `json:"end"`
	MarginBefore *int  `json:"marginBefore"`
	MarginAfter  *int  `json:"marginAfter"`
	// EventID is the EPG event being recorded, if known (e.g. from the
	// guide) — passed on so the box links the timer to it.
	EventID int64 `json:"eventId"`
}

// TimerPreview is how a new timer appears in a plan.
type TimerPreview struct {
	ServiceReference string `json:"serviceReference"`
	ServiceName      string `json:"serviceName"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Begin            int64  `json:"begin"`
	End              int64  `json:"end"`
	MarginBefore     int    `json:"marginBefore"`
	MarginAfter      int    `json:"marginAfter"`
	RecordBegin      int64  `json:"recordBegin"`
	RecordEnd        int64  `json:"recordEnd"`
}

func (req *createRequest) validate(now time.Time) map[string]httpx.FieldError {
	errs := map[string]httpx.FieldError{}
	req.Name = strings.TrimSpace(req.Name)
	if strings.TrimSpace(req.ServiceReference) == "" {
		errs["serviceReference"] = httpx.FieldError{Code: "required"}
	}
	if req.Name == "" {
		errs["name"] = httpx.FieldError{Code: "required"}
	} else if len([]rune(req.Name)) > 200 {
		errs["name"] = httpx.FieldError{Code: "too_long", Params: map[string]any{"maxLength": 200}}
	}
	switch {
	case req.Begin <= 0 || req.End <= 0:
		errs["begin"] = httpx.FieldError{Code: "required"}
	case req.End <= req.Begin:
		errs["end"] = httpx.FieldError{Code: "timer.endBeforeBegin"}
	case req.End-req.Begin > 24*60*60:
		errs["end"] = httpx.FieldError{Code: "timer.tooLong", Params: map[string]any{"maxHours": 24}}
	case req.End <= now.Unix():
		errs["end"] = httpx.FieldError{Code: "timer.inPast"}
	}
	for key, m := range map[string]*int{"marginBefore": req.MarginBefore, "marginAfter": req.MarginAfter} {
		if m != nil && (*m < 0 || *m > MaxMargin) {
			errs[key] = httpx.FieldError{Code: "out_of_range", Params: map[string]any{"min": 0, "max": MaxMargin}}
		}
	}
	return errs
}

// fingerprint identifies a timer list independent of order.
func fingerprint(timers []webcontrol.Timer) string {
	sorted := make([]webcontrol.Timer, len(timers))
	copy(sorted, timers)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Begin != b.Begin {
			return a.Begin < b.Begin
		}
		if a.ServiceReference != b.ServiceReference {
			return a.ServiceReference < b.ServiceReference
		}
		return a.Name < b.Name
	})
	b, _ := json.Marshal(sorted)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// occupiesTuner reports whether a timer will (still) need a tuner.
func occupiesTuner(t webcontrol.Timer) bool { return !t.Disabled && t.State != webcontrol.TimerEnded }

// conflicts returns the timers that, together with a new recording of
// serviceRef from begin to end, likely need more tuners than the box has —
// transponder- and delivery-system-aware (internal/tuner). An estimate:
// the box runs its own exact check on apply.
func conflicts(timers []webcontrol.Timer, info *webcontrol.DeviceInfo, serviceRef string, begin, end int64) []webcontrol.Timer {
	var candidates []webcontrol.Timer
	var bookings []tuner.Booking
	for _, t := range timers {
		if occupiesTuner(t) {
			candidates = append(candidates, t)
			bookings = append(bookings, tuner.Booking{ServiceReference: t.ServiceReference, Begin: t.Begin, End: t.End})
		}
	}
	var out []webcontrol.Timer
	for _, i := range tuner.Conflicts(tuner.Frontends(info), bookings, tuner.Booking{ServiceReference: serviceRef, Begin: begin, End: end}) {
		out = append(out, candidates[i])
	}
	return out
}

// findTimer returns the timer the box would match for ref.
func findTimer(timers []webcontrol.Timer, ref webcontrol.TimerRef) (webcontrol.Timer, bool) {
	for _, t := range timers {
		if t.ServiceReference == ref.ServiceReference && t.Begin == ref.Begin && t.End == ref.End {
			return t, true
		}
	}
	return webcontrol.Timer{}, false
}

// boxFailure turns a write error into a plan.Failure where there's
// something specific to tell the user.
func boxFailure(err error) error {
	var rej *webcontrol.RejectedError
	if errors.As(err, &rej) {
		return &plan.Failure{Status: http.StatusConflict, Code: CodeBoxRejected, Message: "the box refused the change; nothing was changed", Details: map[string]any{"boxText": rej.Text}, Err: err}
	}
	return err
}

func (h *Timer) planCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	var req createRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "expected a timer JSON object")
		return
	}
	if errs := req.validate(h.Now()); len(errs) > 0 {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "the timer is invalid; nothing was planned", errs)
		return
	}

	ctx := r.Context()
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	info, _ := h.WC.DeviceInfo(ctx)

	before, after := DefaultMarginBefore, DefaultMarginAfter
	if req.MarginBefore != nil {
		before = *req.MarginBefore
	}
	if req.MarginAfter != nil {
		after = *req.MarginAfter
	}
	p := TimerPreview{
		ServiceReference: req.ServiceReference, ServiceName: req.ServiceName, Name: req.Name, Description: req.Description,
		Begin: req.Begin, End: req.End, MarginBefore: before, MarginAfter: after,
		RecordBegin: req.Begin - int64(before)*60, RecordEnd: req.End + int64(after)*60,
	}

	var warnings []status.Problem
	// The box's own duplicate rule: same channel, same begin — end doesn't
	// matter. It drops such a timer silently (ARCHITECTURE.md).
	for _, t := range timers {
		if t.Begin == p.RecordBegin && tuner.SameService(t.ServiceReference, p.ServiceReference) {
			warnings = append(warnings, status.Problem{Code: CodeTimerDuplicate, Params: map[string]any{"name": t.Name}})
		}
	}
	if clash := conflicts(timers, info, p.ServiceReference, p.RecordBegin, p.RecordEnd); len(clash) > 0 {
		with := make([]map[string]any, 0, len(clash))
		for _, t := range clash {
			with = append(with, map[string]any{"name": t.Name, "serviceName": t.ServiceName, "begin": t.Begin, "end": t.End})
		}
		// "estimate": the box runs its own, exact check on apply.
		warnings = append(warnings, status.Problem{Code: CodeTimerConflict, Params: map[string]any{"with": with, "tuners": info.Tuners(), "estimate": true}})
	}

	pl := h.Plans.Create(user, plan.Draft{
		Action:      "timer.create",
		Changes:     []plan.Change{{Type: "timer.create", After: p}},
		Warnings:    warnings,
		Summary:     "Created timer: " + p.Name + " (" + p.ServiceName + ")",
		Detail:      p,
		Fingerprint: fingerprint(timers),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.WC.TimerList(ctx)
			if err != nil {
				return "", err
			}
			return fingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			if err := h.WC.AddTimer(ctx, webcontrol.NewTimer{
				ServiceReference: p.ServiceReference, Name: p.Name, Description: p.Description,
				Begin: p.RecordBegin, End: p.RecordEnd, EventID: req.EventID,
			}); err != nil {
				return boxFailure(err)
			}
			// Verify, don't trust: the box reports success for a timer it
			// silently dropped (ARCHITECTURE.md).
			after, err := h.WC.TimerList(ctx)
			if err != nil {
				return err
			}
			made, ok := findTimer(after, webcontrol.TimerRef{ServiceReference: p.ServiceReference, Begin: p.RecordBegin, End: p.RecordEnd})
			if !ok {
				return &plan.Failure{Status: http.StatusConflict, Code: CodeTimerNotCreated, Message: "the box reported success, but the timer isn't in its list — usually one on the same channel with the same start already exists"}
			}
			if h.Owners != nil {
				// Best effort: without the record only an admin can delete it.
				_ = h.Owners.SetTimerOwner(ctx, made.ServiceReference, made.Begin, user.ID)
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

type deleteRequest struct {
	ServiceReference string `json:"serviceReference"`
	Begin            int64  `json:"begin"`
	End              int64  `json:"end"`
}

// planDelete previews deleting a timer, identified exactly as
// GET /api/v1/timers lists it (the box matches service reference, begin
// and end verbatim).
func (h *Timer) planDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := httpx.User(r.Context())
	var req deleteRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || req.ServiceReference == "" || req.Begin <= 0 || req.End <= 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"serviceReference", "begin", "end"} as listed by GET /api/v1/timers`)
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
		httpx.WriteError(w, r, http.StatusForbidden, CodeTimerNotYours, "this profile may only delete timers it made")
		return
	}

	var warnings []status.Problem
	if t.Active() {
		warnings = append(warnings, status.Problem{Code: CodeTimerDeleteRunning, Params: map[string]any{"name": t.Name}})
	}
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "timer.delete",
		Changes:     []plan.Change{{Type: "timer.delete", Before: t}},
		Warnings:    warnings,
		Summary:     "Deleted timer: " + t.Name + " (" + t.ServiceName + ")",
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
			after, err := h.WC.TimerList(ctx)
			if err != nil {
				return err
			}
			if _, still := findTimer(after, ref); still {
				return &plan.Failure{Status: http.StatusConflict, Code: CodeTimerNotDeleted, Message: "the box reported success, but the timer is still in its list"}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// IsWritesDisabled reports whether err means device writes are disabled.
func IsWritesDisabled(err error) bool { return errors.Is(err, webcontrol.ErrWritesDisabled) }
