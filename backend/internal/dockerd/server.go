package dockerd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
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
	// Policy is which host paths a container may mount; the zero value is
	// DefaultPolicy (/media only).
	Policy MountPolicy
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
	mux.HandleFunc("GET /containers/{id}/image-digest", s.handleImageDigest)
	mux.HandleFunc("GET /containers/{id}/attach", s.handleAttach)
	mux.HandleFunc("GET /images/unused", s.handleUnusedImages)
	mux.HandleFunc("POST /images/prune", s.handlePruneImages)
	return mux
}

func writeErr(w http.ResponseWriter, err error) {
	var se *SpecError
	switch {
	case errors.As(err, &se):
		// 422: the request was understood and refused; the body says why
		// (code and message) so the caller can tell the person.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": se.Code, "message": se.Message, "path": se.Path})
	case errors.Is(err, ErrInvalidService):
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	Service       string            `json:"service"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	Env           map[string]string `json:"env"`
	Cmd           []string          `json:"cmd"`
	HostPort      int               `json:"hostPort"`
	ContainerPort int               `json:"containerPort"`
	Mounts        []Mount           `json:"mounts"`
	Elevated      Elevated          `json:"elevated"`
	MemoryLimit   int64             `json:"memoryLimitBytes"`
	HostNetwork   bool              `json:"hostNetwork"`
	Root          string            `json:"root"`
}

func (s Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // an unknown field is a caller asking for something this surface doesn't offer
	var body createRequest
	if err := dec.Decode(&body); err != nil || body.AppID == "" || body.Image == "" || body.MemoryLimit <= 0 || !ValidService(body.Service) || !ValidRoot(body.Root) {
		http.Error(w, "dockerd: create: appId, image and a positive memoryLimitBytes are required, service is lowercase letters/digits", http.StatusBadRequest)
		return
	}
	spec := ContainerSpec{
		AppID: body.AppID, Service: body.Service, Name: body.Name, Image: body.Image, Env: body.Env, Cmd: body.Cmd,
		HostPort: body.HostPort, ContainerPort: body.ContainerPort,
		Mounts: body.Mounts, Elevated: body.Elevated,
		MemoryLimitBytes: body.MemoryLimit, HostNetwork: body.HostNetwork, Root: body.Root,
	}
	if err := ValidateSpec(spec, s.Policy); err != nil {
		writeErr(w, err)
		return
	}
	c, err := s.Client.Create(r.Context(), spec)
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

func (s Server) handleImageDigest(w http.ResponseWriter, r *http.Request) {
	digest, err := s.Client.ImageDigest(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"digest": digest})
}

// handleAttach is the helper's own loopback half of AppsHub's
// console (App5): it calls the real Client.Attach (Exec, against the
// socket) and then hijacks its own HTTP connection so DreamHub's main
// backend sees the exact same raw-byte-stream contract one hop further
// out, through HTTPClient.Attach — DreamHub's backend talks to this
// helper exactly the way this helper talks to the daemon, no new
// protocol layered in between.
func (s Server) handleAttach(w http.ResponseWriter, r *http.Request) {
	upstream, err := s.Client.Attach(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer upstream.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "dockerd: attach: hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, "dockerd: attach: hijack failed", http.StatusInternalServerError)
		return
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")); err != nil {
		return
	}

	var once sync.Once
	stop := func() { once.Do(func() { conn.Close(); upstream.Close() }) }
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(conn, upstream); stop() }()
	go func() { defer wg.Done(); io.Copy(upstream, conn); stop() }()
	wg.Wait()
}

func (s Server) handleUnusedImages(w http.ResponseWriter, r *http.Request) {
	list, err := s.Client.UnusedImages(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []Image{}
	}
	writeJSON(w, list)
}

// handlePruneImages takes no body at all: there is nothing a caller could
// select, so anything sent is refused rather than ignored.
func (s Server) handlePruneImages(w http.ResponseWriter, r *http.Request) {
	var one [1]byte
	if n, _ := io.ReadFull(http.MaxBytesReader(w, r.Body, 1), one[:]); n > 0 || r.URL.RawQuery != "" {
		http.Error(w, "dockerd: prune images: takes no parameters", http.StatusBadRequest)
		return
	}
	res, err := s.Client.PruneImages(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, res)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
