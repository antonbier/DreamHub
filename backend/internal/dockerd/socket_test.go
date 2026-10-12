package dockerd

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// fakeDaemon stands in for the real dockerd, listening on a unix socket
// exactly like the real one does, so Exec (the only piece of this
// package that ever talks to an actual socket) is exercised for real
// rather than assumed correct from the HTTP-shape docs alone.
func fakeDaemon(t *testing.T) (sockPath string, close func()) {
	t.Helper()
	dir := t.TempDir()
	sockPath = filepath.Join(dir, "docker.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]dockerContainer{
			{ID: "abc123", Names: []string{"/pihole"}, Image: "linuxserver/pihole", State: "running", Status: "Up 1 hour",
				Labels: map[string]string{AppLabel: "pihole"}},
			{ID: "def456", Names: []string{"/other"}, Image: "nginx", State: "running", Status: "Up 1 hour",
				Labels: map[string]string{}}, // no AppLabel — must never surface
		})
	})
	mux.HandleFunc("GET /containers/abc123/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Image":"sha256:imageid1","Config":{"Labels":{"dreamhub.app":"pihole"}}}`))
	})
	mux.HandleFunc("GET /images/sha256:imageid1/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"RepoDigests":["pihole/pihole@sha256:newdigest"]}`))
	})
	mux.HandleFunc("POST /containers/abc123/exec", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"Id":"execid1"}`))
	})
	mux.HandleFunc("POST /exec/execid1/start", func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\nhello from the mock shell"))
	})
	mux.HandleFunc("GET /containers/def456/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Config":{"Labels":{}}}`))
	})
	mux.HandleFunc("GET /containers/missing/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("POST /containers/abc123/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /containers/abc123/logs", func(w http.ResponseWriter, r *http.Request) {
		// One framed stdout line ("hi\n"), Docker's own multiplexed
		// format for a non-tty container.
		frame := []byte{1, 0, 0, 0, 0, 0, 0, 3, 'h', 'i', '\n'}
		_, _ = w.Write(frame)
	})
	// Create/Remove fixtures: "sonarr" doesn't exist yet (so Create's own
	// pre-check passes), "has-container" already does (so Create refuses
	// it), and "bad-image" fails mid-pull the way Docker really reports
	// it — a 200 whose streamed body still carries an "error" line.
	mux.HandleFunc("GET /containers/dreamhub-app-sonarr/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("GET /containers/dreamhub-app-has-container/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Config":{"Labels":{"dreamhub.app":"has-container"}}}`))
	})
	mux.HandleFunc("GET /containers/dreamhub-app-bad-image/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("POST /images/create", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fromImage") == "bad/image" {
			_, _ = w.Write([]byte(`{"status":"pulling"}` + "\n" + `{"error":"manifest unknown"}` + "\n"))
			return
		}
		_, _ = w.Write([]byte(`{"status":"pull complete"}` + "\n"))
	})
	mux.HandleFunc("POST /containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"Id":"newcontainer123"}`))
	})
	mux.HandleFunc("POST /containers/newcontainer123/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /containers/abc123/stop", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /containers/abc123", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &httptest.Server{Listener: listener, Config: &http.Server{Handler: mux}}
	srv.Start()
	return sockPath, srv.Close
}

func TestExec_ListFiltersMissingLabel(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	list, err := e.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].AppID() != "pihole" {
		t.Fatalf("got %+v, want exactly the AppLabel-tagged container", list)
	}
}

func TestExec_RequireOwned(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}
	ctx := context.Background()

	if err := e.Start(ctx, "abc123"); err != nil {
		t.Fatalf("owned container: got %v", err)
	}
	if err := e.Start(ctx, "def456"); err != ErrNotFound {
		t.Fatalf("unlabeled container: got %v, want ErrNotFound", err)
	}
	if err := e.Start(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("nonexistent container: got %v, want ErrNotFound", err)
	}
}

func TestExec_LogsDemuxed(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	out, err := e.Logs(context.Background(), "abc123", 10)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "hi\n" {
		t.Fatalf("got %q, want the frame header stripped to %q", out, "hi\n")
	}
}

func TestExec_Unreachable(t *testing.T) {
	e := Exec{SocketPath: "/nonexistent/docker.sock"}
	if _, err := e.List(context.Background()); err == nil {
		t.Fatal("want an error when the socket doesn't exist")
	}
}

func TestExec_Create(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	c, err := e.Create(context.Background(), ContainerSpec{
		AppID: "sonarr", Image: "linuxserver/sonarr:latest",
		Env: map[string]string{"TZ": "Europe/Rome"}, HostPort: 8989, ContainerPort: 8989,
		HostDir: "/media/hdd/dreamhub-apps/sonarr", ContainerDir: "/config",
		MemoryLimitBytes: 256 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "newcontainer123" || c.AppID() != "sonarr" || c.HostPort != 8989 {
		t.Fatalf("got %+v", c)
	}
}

func TestExec_CreateRefusesExisting(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	_, err := e.Create(context.Background(), ContainerSpec{AppID: "has-container", Image: "x", MemoryLimitBytes: 1})
	if err != ErrAlreadyExists {
		t.Fatalf("got %v, want ErrAlreadyExists", err)
	}
}

func TestExec_CreateFailedPull(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	_, err := e.Create(context.Background(), ContainerSpec{AppID: "bad-image", Image: "bad/image", MemoryLimitBytes: 1})
	if err == nil {
		t.Fatal("want an error when the daemon reports a failed pull")
	}
}

func TestExec_Remove(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	if err := e.Remove(context.Background(), "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := e.Remove(context.Background(), "def456"); err != ErrNotFound {
		t.Fatalf("unlabeled container: got %v, want ErrNotFound", err)
	}
}

func TestExec_ImageDigest(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	digest, err := e.ImageDigest(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if digest != "sha256:newdigest" {
		t.Fatalf("got %q, want the repo digest with the image name stripped", digest)
	}

	if _, err := e.ImageDigest(context.Background(), "def456"); err != ErrNotFound {
		t.Fatalf("unlabeled container: got %v, want ErrNotFound", err)
	}
}

func TestExec_Attach(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	conn, err := e.Attach(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "hello from the mock shell" {
		t.Fatalf("got %q, want the raw bytes the daemon wrote after the upgrade", got)
	}

	if _, err := e.Attach(context.Background(), "def456"); err != ErrNotFound {
		t.Fatalf("unlabeled container: got %v, want ErrNotFound", err)
	}
}

func TestDemux_PlainFallsThrough(t *testing.T) {
	// A tty-attached container's logs aren't framed at all; demux must
	// not mangle them.
	plain := []byte("already plain text\n")
	if got := demux(plain); string(got) != string(plain) {
		t.Fatalf("got %q, want unchanged %q", got, plain)
	}
}
