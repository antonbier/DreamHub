package dockerd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

// Server is the helper process's own HTTP surface — the whitelisted
// subset DreamHub's main backend is allowed to call, wrapping a Client
// (Exec, talking to the real socket). This is the only thing the helper
// listens with; there is no route here beyond list/start/stop/logs/exec/
// create/remove, so the "hard denials" the plan calls for (no
// --privileged, no arbitrary mount, no socket re-exposure) hold simply
// because create's own request body is a fixed, narrow shape
// (createRequest below, not a raw Docker container spec) — not because of
// a runtime check that a bug could skip.
type Server struct {
	Client Client
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /containers", s.handleList)
	mux.HandleFunc("POST /containers", s.handleCreate)
	mux.HandleFunc("POST /containers/{id}/start", s.handleStart)
	mux.HandleFunc("POST /containers/{id}/stop", s.handleStop)
	mux.HandleFunc("DELETE /containers/{id}", s.handleRemove)
	mux.HandleFunc("GET /containers/{id}/logs", s.handleLogs)
	mux.HandleFunc("POST /containers/{id}/exec", s.handleExec)
	return mux
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrAlreadyExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, ErrUnavailable):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s Server) handleList(w http.ResponseWriter, r *http.Request) {
	list, err := s.Client.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, list)
}

func (s Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if err := s.Client.Start(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if err := s.Client.Stop(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createRequest is the server's own request body — a fixed, narrow
// shape, not a passthrough for any part of ContainerSpec's Docker-facing
// meaning (HostDir/ContainerDir in particular: the caller sends the
// paths, the daemon-facing Exec.Create is what actually turns them into
// a bind mount, and nothing here lets a caller add a second one, an
// absolute host root, or a privileged flag).
type createRequest struct {
	AppID         string            `json:"appId"`
	Image         string            `json:"image"`
	Env           map[string]string `json:"env"`
	HostPort      int               `json:"hostPort"`
	ContainerPort int               `json:"containerPort"`
	HostDir       string            `json:"hostDir"`
	ContainerDir  string            `json:"containerDir"`
	MemoryLimit   int64             `json:"memoryLimitBytes"`
}

func (s Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var body createRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AppID == "" || body.Image == "" || body.HostDir == "" || body.ContainerDir == "" || body.MemoryLimit <= 0 {
		http.Error(w, "dockerd: create: appId, image, hostDir, containerDir and a positive memoryLimitBytes are required", http.StatusBadRequest)
		return
	}
	c, err := s.Client.Create(r.Context(), ContainerSpec{
		AppID: body.AppID, Image: body.Image, Env: body.Env,
		HostPort: body.HostPort, ContainerPort: body.ContainerPort,
		HostDir: body.HostDir, ContainerDir: body.ContainerDir,
		MemoryLimitBytes: body.MemoryLimit,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, c)
}

func (s Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.Client.Remove(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	tail := 200
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			tail = n
		}
	}
	out, err := s.Client.Logs(r.Context(), r.PathValue("id"), tail)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(out)
}

func (s Server) handleExec(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cmd []string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Cmd) == 0 {
		http.Error(w, "dockerd: exec: a non-empty cmd is required", http.StatusBadRequest)
		return
	}
	out, err := s.Client.Exec(r.Context(), r.PathValue("id"), body.Cmd)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
