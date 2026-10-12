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
		_, _ = w.Write([]byte(`{"Config":{"Labels":{"dreamhub.app":"pihole"}}}`))
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

func TestDemux_PlainFallsThrough(t *testing.T) {
	// A tty-attached container's logs aren't framed at all; demux must
	// not mangle them.
	plain := []byte("already plain text\n")
	if got := demux(plain); string(got) != string(plain) {
		t.Fatalf("got %q, want unchanged %q", got, plain)
	}
}
