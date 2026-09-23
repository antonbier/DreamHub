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

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Timer is TimerHub: listing timers and creating them via preview-and-confirm.
type Timer struct {
	WC    webcontrol.Client
	Plans *plan.Service
	Now   func() time.Time
}

// Mount registers TimerHub's routes.
func (h *Timer) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/timers", h.list)
	mux.HandleFunc("POST /api/v1/timers/plan", h.planCreate)
}

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
)

func (h *Timer) list(w http.ResponseWriter, r *http.Request) {
	timers, err := h.WC.TimerList(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	info, _ := h.WC.DeviceInfo(r.Context())
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"timers": timers, "tuners": info.Tuners()})
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

func active(t webcontrol.Timer) bool { return !t.Disabled && t.State != 3 }

// conflicts returns the active timers overlapping [begin, end) if recording
// them together with the new timer would need more tuners than the box has.
func conflicts(timers []webcontrol.Timer, begin, end int64, tuners int) []webcontrol.Timer {
	var overlapping []webcontrol.Timer
	for _, t := range timers {
		if active(t) && t.Begin < end && begin < t.End {
			overlapping = append(overlapping, t)
		}
	}
	// Peak concurrency within the new timer's window: sweep over the
	// overlapping timers' starts and ends.
	type ev struct {
		at    int64
		delta int
	}
	var evs []ev
	for _, t := range overlapping {
		evs = append(evs, ev{max(t.Begin, begin), +1}, ev{min(t.End, end), -1})
	}
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].at != evs[j].at {
			return evs[i].at < evs[j].at
		}
		return evs[i].delta < evs[j].delta // ends before starts at the same instant
	})
	peak, cur := 0, 0
	for _, e := range evs {
		cur += e.delta
		peak = max(peak, cur)
	}
	if peak+1 > tuners {
		return overlapping
	}
	return nil
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
	for _, t := range timers {
		if t.ServiceReference == p.ServiceReference && t.Begin == p.RecordBegin && t.End == p.RecordEnd {
			warnings = append(warnings, status.Problem{Code: CodeTimerDuplicate, Params: map[string]any{"name": t.Name}})
		}
	}
	if clash := conflicts(timers, p.RecordBegin, p.RecordEnd, info.Tuners()); len(clash) > 0 {
		with := make([]map[string]any, 0, len(clash))
		for _, t := range clash {
			with = append(with, map[string]any{"name": t.Name, "serviceName": t.ServiceName, "begin": t.Begin, "end": t.End})
		}
		warnings = append(warnings, status.Problem{Code: CodeTimerConflict, Params: map[string]any{"with": with, "tuners": info.Tuners()}})
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
			return h.WC.AddTimer(ctx, webcontrol.NewTimer{
				ServiceReference: p.ServiceReference, Name: p.Name, Description: p.Description,
				Begin: p.RecordBegin, End: p.RecordEnd,
			})
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// IsWritesDisabled reports whether err means device writes are disabled.
func IsWritesDisabled(err error) bool { return errors.Is(err, webcontrol.ErrWritesDisabled) }
