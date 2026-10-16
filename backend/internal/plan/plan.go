// Package plan implements preview-and-confirm (PRINCIPLES.md, "Preview and
// confirm — no silent changes"): every change to recordings, timers,
// bouquets or the system is first turned into a plan that lists exactly
// what will happen, and only executed when the client applies that plan by
// ID. No client can skip the preview.
//
// Plans are single use, expire, belong to the user who created them, and are
// rejected as stale if the state they were computed from has changed in the
// meantime. They live in memory: after a restart the user simply previews
// again.
package plan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/antonbier/DreamHub/backend/internal/box"
	"log/slog"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// TTL is how long a plan can be applied after it was created.
const TTL = 10 * time.Minute

// Errors returned by Apply.
var (
	ErrNotFound = errors.New("plan: not found, expired or already used")
	ErrNotOwner = errors.New("plan: belongs to another user")
	ErrStale    = errors.New("plan: state changed since the preview")
)

// Failure is an Execute error a hub wants the client to see with a stable,
// translatable code — e.g. the box refusing a timer, or a write the box
// claimed to make but didn't. Anything else from Execute is reported as a
// generic "apply_failed".
type Failure struct {
	Status  int    // HTTP status
	Code    string // error code (i18n key for the client)
	Message string // English developer hint
	Details any
	Err     error // underlying cause, for logs
}

func (f *Failure) Error() string {
	if f.Err != nil {
		return "plan: " + f.Code + ": " + f.Err.Error()
	}
	return "plan: " + f.Code
}

func (f *Failure) Unwrap() error { return f.Err }

// Change is one item of a preview: what kind of change, and the state before
// and after (nil when not applicable).
type Change struct {
	Type   string `json:"type"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

// Draft is what a hub hands to the plan service.
type Draft struct {
	Action   string           // e.g. "timer.create"
	Changes  []Change         // shown to the user
	Warnings []status.Problem // shown to the user; don't block applying
	// Summary (English) and Detail (structured, no secrets) go into the
	// change log once the plan is applied.
	Summary string
	Detail  any
	// Fingerprint identifies the state the preview was computed from;
	// Current recomputes it at apply time. A mismatch means the preview no
	// longer describes what would happen, and the plan is rejected.
	Fingerprint string
	Current     func(ctx context.Context) (string, error)
	// Execute performs the change.
	Execute func(ctx context.Context) error
}

// Plan is the preview returned to the client.
type Plan struct {
	ID        string           `json:"id"`
	Action    string           `json:"action"`
	CreatedAt time.Time        `json:"createdAt"`
	ExpiresAt time.Time        `json:"expiresAt"`
	Changes   []Change         `json:"changes"`
	Warnings  []status.Problem `json:"warnings"`
	// Summary is Draft.Summary, carried through to the client. Every
	// built-in hub already builds its own preview text locally (an i18n
	// key it owns, with Changes/Warnings as the structured data) and
	// ignores this field — it exists for a caller with no i18n key of its
	// own to render instead, namely an extension (internal/extension):
	// literal text the extension's own process provides, since it can't
	// own a key in DreamHub's frontend locale files.
	Summary string `json:"summary,omitempty"`
}

type entry struct {
	plan   Plan
	userID int64
	box    string // the box the preview was made for (internal/box)
	draft  Draft
}

// Service stores pending plans and applies them.
type Service struct {
	store *store.Store
	now   func() time.Time
	// Logger reports what can't be reported to the client (optional).
	Logger *slog.Logger

	mu      sync.Mutex
	pending map[string]entry
}

// NewService returns a plan service recording applied plans in st.
func NewService(st *store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: st, now: now, pending: map[string]entry{}}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Create stores a draft as a pending plan for u and returns its preview.
// The plan belongs to the box ctx is addressed to: applying it acts on
// that box, whatever box the apply request names.
func (s *Service) Create(ctx context.Context, u store.User, d Draft) Plan {
	now := s.now().UTC()
	p := Plan{ID: newID(), Action: d.Action, CreatedAt: now, ExpiresAt: now.Add(TTL), Changes: d.Changes, Warnings: d.Warnings, Summary: d.Summary}
	if p.Changes == nil {
		p.Changes = []Change{}
	}
	if p.Warnings == nil {
		p.Warnings = []status.Problem{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, e := range s.pending { // drop expired plans; the map stays tiny
		if !now.Before(e.plan.ExpiresAt) {
			delete(s.pending, id)
		}
	}
	s.pending[p.ID] = entry{plan: p, userID: u.ID, box: box.ID(ctx), draft: d}
	return p
}

// Result is what an applied plan produced.
type Result struct {
	PlanID string `json:"planId"`
	Action string `json:"action"`
}

// Apply executes a pending plan. The plan is consumed whether execution
// succeeds or not — a failed or stale plan needs a fresh preview.
func (s *Service) Apply(ctx context.Context, u store.User, id string) (Result, error) {
	s.mu.Lock()
	e, ok := s.pending[id]
	if ok && e.userID != u.ID {
		s.mu.Unlock()
		return Result{}, ErrNotOwner
	}
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()
	if !ok || !s.now().Before(e.plan.ExpiresAt) {
		return Result{}, ErrNotFound
	}

	// Once confirmed, a change runs to its end even if the client goes
	// away (a phone locking its screen): stopping a multi-step change
	// halfway — or its own rollback — would leave the box half-changed.
	// The clients underneath keep their own per-request timeouts.
	ctx = context.WithoutCancel(ctx)
	if e.box != "" {
		ctx = box.With(ctx, e.box)
	}
	if e.draft.Current != nil {
		fp, err := e.draft.Current(ctx)
		if err != nil {
			return Result{}, err
		}
		if fp != e.draft.Fingerprint {
			return Result{}, ErrStale
		}
	}
	if err := e.draft.Execute(ctx); err != nil {
		return Result{}, err
	}

	detail, err := json.Marshal(map[string]any{"planId": id, "changes": e.plan.Changes, "detail": e.draft.Detail})
	if err != nil {
		return Result{}, err
	}
	// The change happened: a change-log entry that can't be written must
	// not turn it into a reported failure (the user would retry it).
	if err := s.store.AppendChange(ctx, store.ChangeEntry{
		UserID: u.ID, Action: e.draft.Action, Summary: e.draft.Summary, Detail: detail,
	}); err != nil && s.Logger != nil {
		s.Logger.Error("writing the change log failed", "action", e.draft.Action, "plan", id, "error", err)
	}
	return Result{PlanID: id, Action: e.draft.Action}, nil
}
