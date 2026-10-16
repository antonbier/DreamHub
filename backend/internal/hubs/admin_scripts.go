package hubs

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// AdminHub's script library (docs/ADMINHUB_PLAN.md, A3, #47): scripts an
// admin uploads and may run by hand, each run gated by the usual
// preview-and-confirm and recorded in the change log. AutomationHub later
// triggers the same stored scripts rather than keeping its own copies.

// maxScriptSize bounds what's stored — plenty for an admin shell/python
// script, small enough it can't bloat the one SQLite file (PRINCIPLES.md).
const maxScriptSize = 256 * 1024

type scriptView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	UploadedAt string `json:"uploadedAt"`
}

func toScriptView(s store.Script) scriptView {
	return scriptView{ID: s.ID, Name: s.Name, UploadedAt: s.UploadedAt.UTC().Format(time.RFC3339)}
}

func (h *Admin) listScripts(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.Scripts(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load the scripts")
		return
	}
	out := make([]scriptView, 0, len(list))
	for _, s := range list {
		out = append(out, toScriptView(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"scripts": out})
}

func (h *Admin) uploadScript(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Name == "" || body.Content == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name": "...", "content": "..."}`)
		return
	}
	if len(body.Content) > maxScriptSize {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "the script is too large",
			map[string]httpx.FieldError{"content": {Code: "too_large", Params: map[string]any{"max": maxScriptSize}}})
		return
	}
	user, _ := httpx.User(r.Context())
	id, err := h.Store.AddScript(r.Context(), body.Name, body.Content, user.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not store the script")
		return
	}
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "admin.script.upload", Summary: "Uploaded script " + body.Name})
	httpx.WriteJSON(w, http.StatusCreated, toScriptView(store.Script{ID: id, Name: body.Name, UploadedAt: h.now(), UploadedBy: user.ID}))
}

func (h *Admin) scriptID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func (h *Admin) deleteScript(w http.ResponseWriter, r *http.Request) {
	id, ok := h.scriptID(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such script")
		return
	}
	s, found, err := h.Store.Script(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not look up the script")
		return
	}
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such script")
		return
	}
	if err := h.Store.DeleteScript(r.Context(), id); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not delete the script")
		return
	}
	user, _ := httpx.User(r.Context())
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "admin.script.delete", Summary: "Deleted script " + s.Name})
	w.WriteHeader(http.StatusNoContent)
}

// scriptFingerprint is what a run plan was previewed against: the script
// must still exist, unchanged, when it's actually run.
func scriptFingerprint(s store.Script) string {
	return strconv.FormatInt(s.ID, 10) + "|" + s.Content
}

func (h *Admin) planRunScript(w http.ResponseWriter, r *http.Request) {
	id, ok := h.scriptID(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such script")
		return
	}
	if h.Scripts == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "scripts_unavailable", "running scripts isn't available on this system")
		return
	}
	s, found, err := h.Store.Script(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not look up the script")
		return
	}
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such script")
		return
	}
	user, _ := httpx.User(r.Context())
	pl := h.Plans.Create(r.Context(), user, plan.Draft{
		Action:      "script.run",
		Changes:     []plan.Change{{Type: "script.run", After: map[string]any{"name": s.Name}}},
		Summary:     "Run script " + s.Name,
		Detail:      map[string]any{"scriptId": s.ID, "name": s.Name},
		Fingerprint: scriptFingerprint(s),
		Current: func(ctx context.Context) (string, error) {
			now, found, err := h.Store.Script(ctx, id)
			if err != nil || !found {
				return "", err
			}
			return scriptFingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			started := h.now()
			output, exitCode, runErr := h.Scripts.Run(ctx, s.Content)
			finished := h.now()
			// Recorded even on a start failure (runErr != nil, exitCode
			// -1) — an admin asking "did my last attempt even run" should
			// always get an answer.
			_ = h.Store.AddScriptRun(context.WithoutCancel(ctx), store.ScriptRun{
				ScriptID: s.ID, StartedAt: started, FinishedAt: finished, ExitCode: exitCode, Output: output, UserID: user.ID,
			})
			_ = h.Store.AppendChange(context.WithoutCancel(ctx), store.ChangeEntry{
				UserID: user.ID, Action: "admin.script.run", Summary: "Ran script " + s.Name,
			})
			if runErr != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "script_run_failed", Message: "the script could not be started", Err: runErr}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

type scriptRunView struct {
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	ExitCode   int    `json:"exitCode"`
	Output     string `json:"output"`
}

func (h *Admin) scriptRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := h.scriptID(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such script")
		return
	}
	runs, err := h.Store.ScriptRuns(r.Context(), id, 20)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load the script's runs")
		return
	}
	out := make([]scriptRunView, 0, len(runs))
	for _, run := range runs {
		out = append(out, scriptRunView{
			StartedAt: run.StartedAt.UTC().Format(time.RFC3339), FinishedAt: run.FinishedAt.UTC().Format(time.RFC3339),
			ExitCode: run.ExitCode, Output: run.Output,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"runs": out})
}
