package hubs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/journald"
	"github.com/antonbier/DreamHub/backend/internal/logging"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/systemd"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Admin is AdminHub: DreamHub's runtime log level, journald's log viewer
// and enigma2's crash logs (docs/archive/V1_PLAN.md, WP6). Every route requires the
// admin role.
type Admin struct {
	Store *store.Store
	Level *logging.Level
	Logs  journald.Reader
	// FS is the same file system MediaHub's file operations use
	// (internal/recfs) — reused here read-only, rooted the same way, just
	// for a different directory (crashLogDir, not the recording folders).
	FS recfs.FS
	// Systemd reads and restarts the box's services (admin_services.go);
	// nil hides them.
	Systemd systemd.Manager
	// Services overrides DefaultServices (tests).
	Services []string
	// Plans holds restart previews; WC tells which recordings a restart
	// of enigma2 would cut.
	Plans *plan.Service
	WC    webcontrol.Client
	Now   func() time.Time
	// ReadOnly: the same switch as WebControl writes
	// (DREAMHUB_ENABLE_DEVICE_WRITES=0) — no restarts either.
	ReadOnly bool
}

func (h *Admin) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// crashLogDir is where enigma2 writes its own crash logs — confirmed live,
// ARCHITECTURE.md ("journald is volatile"): one file per crash, named
// "enigma2_crash_<unix-seconds>.log".
const crashLogDir = "/media/hdd"

// Mount registers AdminHub's routes.
func (h *Admin) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/admin/log-level", adminOnly(h.getLevel))
	mux.HandleFunc("PUT /api/v1/admin/log-level", adminOnly(h.putLevel))
	mux.HandleFunc("GET /api/v1/admin/logs", adminOnly(h.logs))
	mux.HandleFunc("GET /api/v1/admin/logs/stream", adminOnly(h.logsStream))
	mux.HandleFunc("GET /api/v1/admin/crashlogs", adminOnly(h.crashLogs))
	mux.HandleFunc("GET /api/v1/admin/crashlogs/{name}", adminOnly(h.crashLog))
	mux.HandleFunc("GET /api/v1/admin/services", adminOnly(h.listServices))
	mux.HandleFunc("POST /api/v1/admin/services/plan", adminOnly(h.planRestart))
}

// DefaultDebugMinutes is how long debug logging lasts unless specified.
const DefaultDebugMinutes = 60

func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, ok := httpx.User(r.Context()); !ok || !perm.For(u).Has(perm.System) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "this profile may not do this")
			return
		}
		next(w, r)
	}
}

func (h *Admin) levelView() map[string]any {
	level, until := h.Level.Get()
	out := map[string]any{"level": level}
	if !until.IsZero() {
		out["until"] = until.Format(time.RFC3339)
	}
	return out
}

func (h *Admin) getLevel(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, h.levelView())
}

func (h *Admin) putLevel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Level   string `json:"level"`
		Minutes int    `json:"minutes"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"level": "debug"|"info", "minutes": 1-1440}`)
		return
	}
	switch body.Level {
	case "debug":
		if body.Minutes == 0 {
			body.Minutes = DefaultDebugMinutes
		}
		if body.Minutes < 1 || body.Minutes > 1440 {
			httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "minutes must be between 1 and 1440",
				map[string]httpx.FieldError{"minutes": {Code: "out_of_range", Params: map[string]any{"min": 1, "max": 1440}}})
			return
		}
		h.Level.SetDebug(time.Duration(body.Minutes) * time.Minute)
	case "info":
		h.Level.SetInfo()
	default:
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "level must be debug or info",
			map[string]httpx.FieldError{"level": {Code: "invalid_option", Params: map[string]any{"options": []string{"debug", "info"}}}})
		return
	}

	// DreamHub's own configuration: recorded, but no plan needed (it doesn't
	// touch recordings, timers or the system).
	u, _ := httpx.User(r.Context())
	view := h.levelView()
	detail, _ := json.Marshal(view)
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{
		UserID: u.ID, Action: "admin.logLevel", Summary: "Set log level to " + body.Level, Detail: detail,
	})
	httpx.WriteJSON(w, http.StatusOK, view)
}

var levelNames = map[string]journald.Level{
	"emerg": journald.LevelEmerg, "alert": journald.LevelAlert, "crit": journald.LevelCrit,
	"err": journald.LevelErr, "warning": journald.LevelWarning, "notice": journald.LevelNotice,
	"info": journald.LevelInfo, "debug": journald.LevelDebug,
}

type logEntryView struct {
	Time    string `json:"time"` // RFC 3339, UTC
	Unit    string `json:"unit,omitempty"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// logs answers AdminHub's log viewer (ROADMAP.md, "Logs from journald").
// Bounded and filtered server-side — the journal itself is the box's whole
// RAM-backed buffer since boot (ARCHITECTURE.md, "journald is volatile"),
// never fetched in full.
// parseLogQuery builds a journald.Query from the request's filters (unit,
// level, text, limit) — shared by the bounded snapshot (logs) and the live
// tail (logsStream), which use the exact same filters.
func parseLogQuery(r *http.Request) (journald.Query, *httpx.FieldError) {
	q := journald.Query{Unit: r.URL.Query().Get("unit"), Text: r.URL.Query().Get("text"), MaxLevel: journald.LevelDebug}
	if lv := r.URL.Query().Get("level"); lv != "" {
		l, ok := levelNames[lv]
		if !ok {
			return q, &httpx.FieldError{Code: "invalid_option"}
		}
		q.MaxLevel = l
	}
	q.Limit = 200
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			return q, &httpx.FieldError{Code: "out_of_range"}
		}
		q.Limit = n
	}
	return q, nil
}

func toLogEntryView(e journald.Entry) logEntryView {
	return logEntryView{Time: e.Time.UTC().Format(time.RFC3339), Unit: e.Unit, Level: e.Level.String(), Message: e.Message}
}

func (h *Admin) logs(w http.ResponseWriter, r *http.Request) {
	q, ferr := parseLogQuery(r)
	if ferr != nil {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "invalid filter",
			map[string]httpx.FieldError{"level": *ferr})
		return
	}
	entries, err := h.Logs.Read(r.Context(), q)
	switch {
	case err == nil:
		out := make([]logEntryView, 0, len(entries))
		for _, e := range entries {
			out = append(out, toLogEntryView(e))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": out})
	case err == journald.ErrUnavailable:
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "journald_unavailable", "journalctl isn't available on this system")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the journal")
	}
}

// logStreamInterval is how often logsStream re-polls journalctl. journald
// has no long-lived "tail -f" a Go client can hold open without shelling
// out to a second, separately-managed `journalctl -f` process per viewer;
// polling the same bounded Read() the snapshot view uses, deduped against
// what's already been sent, gets the same live-tail result far more simply.
const logStreamInterval = 3 * time.Second

// logsStream is AdminHub's live tail (ROADMAP.md: "Live tail over
// WebSocket" — implemented as SSE; see api.handleEventsStream's doc comment
// for why).
func (h *Admin) logsStream(w http.ResponseWriter, r *http.Request) {
	q, ferr := parseLogQuery(r)
	if ferr != nil {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "invalid filter",
			map[string]httpx.FieldError{"level": *ferr})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	seen := map[string]bool{}
	send := func() {
		entries, err := h.Logs.Read(ctx, q)
		if err != nil {
			return
		}
		for _, e := range entries {
			k := e.Time.String() + "|" + e.Unit + "|" + e.Message
			if seen[k] {
				continue
			}
			seen[k] = true
			data, err := json.Marshal(toLogEntryView(e))
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
		}
		flusher.Flush()
	}
	send()
	ticker := time.NewTicker(logStreamInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

type crashLogView struct {
	Name string `json:"name"`
	At   string `json:"at,omitempty"` // RFC 3339, UTC — parsed from the file name when it carries a timestamp
	Size int64  `json:"size"`
}

var crashLogPattern = regexp.MustCompile(`^enigma2_crash_(\d+)\.log$`)

// crashLogs lists enigma2's own crash logs (ROADMAP.md, "List of enigma2
// crash logs"). An empty or missing crash-log directory (no crash yet, or
// not running on the box) is an empty list, not an error.
func (h *Admin) crashLogs(w http.ResponseWriter, r *http.Request) {
	if h.FS == nil { // not running on the box (PRINCIPLES.md: unavailable, not broken)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"logs": []crashLogView{}})
		return
	}
	entries, err := h.FS.ReadDir(crashLogDir)
	if err != nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"logs": []crashLogView{}})
		return
	}
	var out []crashLogView
	for _, e := range entries {
		m := crashLogPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		v := crashLogView{Name: e.Name(), Size: info.Size()}
		if sec, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			v.At = time.Unix(sec, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name }) // most recent crash first
	if out == nil {
		out = []crashLogView{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"logs": out})
}

// secretPattern redacts a "key: value" or "key=value" line whose key looks
// like a credential, so a crash log downloaded for support never carries
// one along — even though enigma2's own crash logs are Python tracebacks
// unlikely to contain one, ROADMAP.md asks for this explicitly.
var secretPattern = regexp.MustCompile(`(?im)^(.*\b(?:password|token|secret|apikey|api_key|authorization)\b\s*[:=]\s*).*$`)

// crashLog returns one crash log's content, secrets redacted.
func (h *Admin) crashLog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !crashLogPattern.MatchString(name) || h.FS == nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such crash log")
		return
	}
	content, err := h.FS.ReadFile(crashLogDir + "/" + name)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such crash log")
		return
	}
	redacted := secretPattern.ReplaceAllString(string(content), "${1}[redacted]")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"name": name, "content": redacted})
}
