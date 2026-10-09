package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/jobs"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// jobView is a background job as a client sees it (internal/jobs). message
// and error are i18n keys, like notifications'.
type jobView struct {
	ID            int64           `json:"id"`
	Kind          string          `json:"kind"`
	Key           string          `json:"key,omitempty"`
	OwnerID       int64           `json:"ownerId"`
	State         string          `json:"state"`
	Done          int64           `json:"done"`
	Total         int64           `json:"total"`
	Message       string          `json:"message,omitempty"`
	MessageParams json.RawMessage `json:"messageParams,omitempty"`
	Error         string          `json:"error,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	StartedAt     *time.Time      `json:"startedAt,omitempty"`
	FinishedAt    *time.Time      `json:"finishedAt,omitempty"`
}

func toJobView(j store.Job) jobView {
	v := jobView{ID: j.ID, Kind: j.Kind, Key: j.Key, OwnerID: j.OwnerID, State: j.State, Done: j.Done, Total: j.Total,
		Message: j.Message, Error: j.Error, CreatedAt: j.CreatedAt}
	if j.Message != "" && len(j.MessageParams) > 0 {
		v.MessageParams = j.MessageParams
	}
	if !j.StartedAt.IsZero() {
		v.StartedAt = &j.StartedAt
	}
	if !j.FinishedAt.IsZero() {
		v.FinishedAt = &j.FinishedAt
	}
	return v
}

// mayTouchJob: a profile sees and cancels its own jobs; perm.System all.
func mayTouchJob(r *http.Request, j store.Job) bool {
	return j.OwnerID == current(r).ID || perms(r).Has(perm.System)
}

// jobFromPath loads {id}, answering 404 for unknown jobs and for other
// profiles' jobs alike, so IDs don't leak what others run.
func jobFromPath(d Deps, w http.ResponseWriter, r *http.Request) (store.Job, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		var j store.Job
		if j, err = d.Store.JobByID(r.Context(), id); err == nil && mayTouchJob(r, j) {
			return j, true
		}
		if err != nil && !errors.Is(err, store.ErrJobNotFound) {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load the job")
			return store.Job{}, false
		}
	}
	httpx.WriteError(w, r, http.StatusNotFound, "job_not_found", "no such job")
	return store.Job{}, false
}

// handleJobs lists the profile's jobs (everyone's with perm.System), active
// ones first, and why the queue is held right now ("" when it isn't).
func handleJobs(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner := current(r).ID
		if perms(r).Has(perm.System) {
			owner = 0
		}
		list, err := d.Store.Jobs(r.Context(), owner, 100)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load jobs")
			return
		}
		out := make([]jobView, 0, len(list))
		for _, j := range list {
			out = append(out, toJobView(j))
		}
		blocked := ""
		if d.Jobs != nil {
			blocked = d.Jobs.BlockedBy()
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"jobs": out, "blockedBy": blocked})
	}
}

func handleJob(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if j, ok := jobFromPath(d, w, r); ok {
			httpx.WriteJSON(w, http.StatusOK, toJobView(j))
		}
	}
}

// handleCancelJob stops a job. Cancelling only stops work: whatever a job
// already changed went through its own plan, and stays.
func handleCancelJob(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		j, ok := jobFromPath(d, w, r)
		if !ok {
			return
		}
		if d.Jobs == nil {
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "jobs_unavailable", "the job queue isn't running")
			return
		}
		j, err := d.Jobs.Cancel(r.Context(), j.ID)
		if errors.Is(err, jobs.ErrNotActive) {
			httpx.WriteError(w, r, http.StatusConflict, "job_finished", "the job has already finished")
			return
		}
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not cancel the job")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toJobView(j))
	}
}
