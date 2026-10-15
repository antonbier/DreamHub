package dockerd

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
		if r.URL.Query().Get("filters") == "" { // every container on the daemon, for the image check
			_, _ = w.Write([]byte(`[{"Id":"abc123","ImageID":"sha256:inuse1"},{"Id":"def456","ImageID":"sha256:inuse1"}]`))
			return
		}
		_ = json.NewEncoder(w).Encode([]dockerContainer{
			{ID: "abc123", Names: []string{"/pihole"}, Image: "linuxserver/pihole", State: "running", Status: "Up 1 hour",
				Labels: map[string]string{AppLabel: "pihole"}},
			{ID: "def456", Names: []string{"/other"}, Image: "nginx", State: "running", Status: "Up 1 hour",
				Labels: map[string]string{}}, // no AppLabel — must never surface
		})
	})
	mux.HandleFunc("GET /containers/abc123/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Image":"sha256:imageid1","State":{"RestartCount":3},"Config":{"Labels":{"dreamhub.app":"pihole"}}}`))
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
		lastCreateBody, _ = io.ReadAll(r.Body)
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

	// Images: "inuse1" is used by a container, "old1" and "dangling1" are not;
	// old1 shares 60 MB of its layers with an image that stays.
	mux.HandleFunc("GET /images/json", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("shared-size") != "1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`[{"Id":"sha256:inuse1","RepoTags":["a/a:1"],"Size":100,"SharedSize":0},` +
			`{"Id":"sha256:old1","RepoTags":["a/a:0"],"Size":100000000,"SharedSize":60000000},` +
			`{"Id":"sha256:dangling1","RepoTags":null,"Size":5000,"SharedSize":-1}]`))
	})
	// Deleting an image by id (the prune walks its own list; Docker's
	// "prune -a" is never called).
	mux.HandleFunc("DELETE /images/{id}", func(w http.ResponseWriter, r *http.Request) {
		deletedImages = append(deletedImages, r.PathValue("id")+"?"+r.URL.RawQuery)
		_, _ = w.Write([]byte(`[{"Untagged":"x"},{"Deleted":"` + r.PathValue("id") + `"}]`))
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
		Root:             "/media/hdd/dreamhub-apps",
		Mounts:           []Mount{{HostDir: "/media/hdd/dreamhub-apps/sonarr", ContainerDir: "/config"}},
		MemoryLimitBytes: 256 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "newcontainer123" || c.AppID() != "sonarr" || c.HostPort != 8989 {
		t.Fatalf("got %+v", c)
	}
}

// lastCreateBody is the most recent create request the fake daemon saw.
var lastCreateBody []byte

func TestExec_CreateHostNetwork(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	c, err := e.Create(context.Background(), ContainerSpec{
		AppID: "vaultwarden", Image: "vaultwarden/server:latest", HostNetwork: true,
		HostPort: 8000, ContainerPort: 8000, MemoryLimitBytes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(lastCreateBody)
	if !strings.Contains(body, `"NetworkMode":"host"`) || !strings.Contains(body, `"dreamhub.port":"8000"`) || strings.Contains(body, `"HostPort"`) {
		t.Fatalf("create body: %s", body)
	}
	if c.HostPort != 8000 {
		t.Fatalf("got %+v", c)
	}
	if _, err := e.Create(context.Background(), ContainerSpec{AppID: "x", Image: "x", HostNetwork: true, HostPort: 8001, ContainerPort: 8000, MemoryLimitBytes: 1}); err == nil {
		t.Fatal("a remapped port on the host network must be refused")
	}
}

// A multi-container app's helper: its own name and ServiceLabel, its Cmd
// passed through, no port label.
func TestExec_CreateService(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}

	c, err := e.Create(context.Background(), ContainerSpec{
		AppID: "nextcloud", Service: "db", Image: "mariadb:11", HostNetwork: true,
		Cmd: []string{"--bind-address=127.0.0.1"}, MemoryLimitBytes: 1, Root: "/media/hdd/dreamhub-apps",
		Mounts: []Mount{
			{HostDir: "/media/hdd/dreamhub-apps/nextcloud.db", ContainerDir: "/var/lib/mysql"},
			{HostDir: "/media/hdd/dreamhub-apps/nextcloud.data", ContainerDir: "/data", ReadOnly: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(lastCreateBody)
	if !strings.Contains(body, `"dreamhub.service":"db"`) || !strings.Contains(body, `"Cmd":["--bind-address=127.0.0.1"]`) || strings.Contains(body, `"dreamhub.port"`) ||
		!strings.Contains(body, `"Binds":["/media/hdd/dreamhub-apps/nextcloud.db:/var/lib/mysql","/media/hdd/dreamhub-apps/nextcloud.data:/data:ro"]`) {
		t.Fatalf("create body: %s", body)
	}
	if c.Name != "dreamhub-app-nextcloud-db" || c.Service() != "db" || c.AppID() != "nextcloud" {
		t.Fatalf("got %+v", c)
	}
	if _, err := e.Create(context.Background(), ContainerSpec{AppID: "x", Service: "DB; rm", Image: "x", MemoryLimitBytes: 1}); err != ErrInvalidService {
		t.Fatalf("got %v, want ErrInvalidService", err)
	}
}

func TestFromDocker_PortFromLabel(t *testing.T) {
	c := fromDocker(dockerContainer{ID: "a", Labels: map[string]string{AppLabel: "vw", PortLabel: "8000"}})
	if c.HostPort != 8000 {
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

// Elevated settings reach Docker's HostConfig only when confirmed, and the
// mounts are checked again at the socket (the second line of defence behind
// the helper's HTTP surface).
func TestExec_CreateElevatedAndRefusals(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	e := Exec{SocketPath: sock}
	base := ContainerSpec{
		AppID: "zigbee", Image: "koenkk/zigbee2mqtt:latest", HostNetwork: true, MemoryLimitBytes: 1, Root: "/media/hdd/dreamhub-apps",
		Mounts: []Mount{{HostDir: "/media/hdd/dreamhub-apps/zigbee", ContainerDir: "/app/data"}},
	}
	spec := base
	spec.Elevated = Elevated{Privileged: true, CapAdd: []string{"NET_ADMIN"}, Devices: []string{"/dev/ttyUSB0:/dev/zigbee:rw"}, PidHost: true, IpcHost: true}
	if _, err := e.Create(context.Background(), spec); code(err) != "needs_confirmation" {
		t.Fatalf("unconfirmed: %v", err)
	}
	spec.Elevated.Confirmed = true
	if _, err := e.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	body := string(lastCreateBody)
	for _, want := range []string{`"Privileged":true`, `"CapAdd":["NET_ADMIN"]`, `"PathOnHost":"/dev/ttyUSB0"`, `"PathInContainer":"/dev/zigbee"`, `"PidMode":"host"`, `"IpcMode":"host"`} {
		if !strings.Contains(body, want) {
			t.Errorf("create body lacks %s: %s", want, body)
		}
	}
	// A plain container carries none of it.
	if _, err := e.Create(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	for _, not := range []string{"Privileged", "CapAdd", "Devices", "PidMode", "IpcMode"} {
		if strings.Contains(string(lastCreateBody), not) {
			t.Errorf("a plain container's body has %s: %s", not, lastCreateBody)
		}
	}
	// A refused mount never gets as far as the daemon.
	lastCreateBody = nil
	bad := base
	bad.Mounts = append([]Mount{}, base.Mounts...)
	bad.Mounts = append(bad.Mounts, Mount{HostDir: "/var/run/docker.sock", ContainerDir: "/var/run/docker.sock"})
	if _, err := e.Create(context.Background(), bad); code(err) != "mount_system" {
		t.Fatalf("socket mount: %v", err)
	}
	if lastCreateBody != nil {
		t.Fatal("the daemon saw a refused create")
	}
}

// deletedImages are the image deletions the fake daemon saw.
var deletedImages []string

func TestExec_UnusedImages(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	imgs, err := Exec{SocketPath: sock}.UnusedImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 || imgs[0].ID != "sha256:old1" || imgs[1].ID != "sha256:dangling1" {
		t.Fatalf("got %+v, want the two images no container uses", imgs)
	}
	if imgs[0].SizeBytes != 40000000 {
		t.Errorf("size %d, want the image's own 40 MB (shared layers stay)", imgs[0].SizeBytes)
	}
	if imgs[1].Tags == nil || imgs[1].SizeBytes != 5000 {
		t.Errorf("dangling image: %+v (tags must be a list, an unknown shared size counts as none)", imgs[1])
	}
}

// Prune removes what UnusedImages lists, one image by id (forced: it is
// unused, and an image with two names would refuse otherwise), nothing a
// caller chose.
func TestExec_PruneImagesRemovesTheUnusedListOnly(t *testing.T) {
	sock, closeFn := fakeDaemon(t)
	defer closeFn()
	deletedImages = nil
	res, err := Exec{SocketPath: sock}.PruneImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 2 || res.ReclaimedBytes != 40000000+5000 {
		t.Fatalf("got %+v", res)
	}
	if len(deletedImages) != 2 || deletedImages[0] != "sha256:old1?force=1" || deletedImages[1] != "sha256:dangling1?force=1" {
		t.Fatalf("deleted %v: the used image must stay", deletedImages)
	}
}
