package dockerd_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/dockerd/mock"
)

// This proves the actual loopback round trip the plan requires: DreamHub's
// main backend (HTTPClient) must only ever see the whitelisted subset the
// helper's Server exposes (mock.Client standing in for the real socket,
// Exec, which only ever runs inside the helper process itself).
func newTestServer(t *testing.T) (dockerd.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(dockerd.Server{Client: mock.New()}.Handler())
	client := dockerd.HTTPClient{BaseURL: srv.URL}
	return client, srv.Close
}

func TestHTTPClient_List(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()

	list, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d containers, want 2", len(list))
	}
	for _, c := range list {
		if c.AppID() == "" {
			t.Errorf("container %s missing AppLabel", c.ID)
		}
	}
}

func TestHTTPClient_StartStop(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()
	ctx := context.Background()

	if err := client.Stop(ctx, "pihole"); err != nil {
		t.Fatal(err)
	}
	list, _ := client.List(ctx)
	if state(list, "pihole") != "exited" {
		t.Fatalf("stop: got state %q", state(list, "pihole"))
	}

	if err := client.Start(ctx, "pihole"); err != nil {
		t.Fatal(err)
	}
	list, _ = client.List(ctx)
	if state(list, "pihole") != "running" {
		t.Fatalf("start: got state %q", state(list, "pihole"))
	}
}

func TestHTTPClient_UnknownContainer(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()
	ctx := context.Background()

	if err := client.Start(ctx, "not-an-app"); err != dockerd.ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
	if _, err := client.Logs(ctx, "not-an-app", 10); err != dockerd.ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestHTTPClient_Logs(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()

	out, err := client.Logs(context.Background(), "adguardhome", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("got empty logs")
	}
}

func TestHTTPClient_Exec(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()

	out, err := client.Exec(context.Background(), "pihole", []string{"pihole", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("got empty exec output")
	}
}

func TestHTTPClient_Unreachable(t *testing.T) {
	// No server at all — the helper being down must look like
	// ErrUnavailable, not a generic error AppsHub would struggle to show.
	client := dockerd.HTTPClient{BaseURL: "http://127.0.0.1:1"}
	if _, err := client.List(context.Background()); !errors.Is(err, dockerd.ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}

func TestServer_RoutesOnlyWhitelistedSubset(t *testing.T) {
	// Nothing beyond list/start/stop/logs/exec/create/remove is reachable
	// — in particular nothing that would let a caller reach the daemon's
	// own raw API (e.g. a passthrough to /containers/{id}/json, which
	// would leak containers outside DreamHub's own apps).
	srv := httptest.NewServer(dockerd.Server{Client: mock.New()}.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/containers/pihole/json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET .../json: got status %d, want 404 (no such route)", resp.StatusCode)
	}
}

func TestHTTPClient_CreateAndRemove(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()
	ctx := context.Background()

	spec := dockerd.ContainerSpec{
		AppID: "sonarr", Image: "linuxserver/sonarr:latest",
		Env:      map[string]string{"TZ": "Europe/Rome"},
		HostPort: 8989, ContainerPort: 8989,
		Root:             "/media/hdd/dreamhub-apps",
		Mounts:           []dockerd.Mount{{HostDir: "/media/hdd/dreamhub-apps/sonarr", ContainerDir: "/config"}},
		MemoryLimitBytes: 256 << 20,
	}
	created, err := client.Create(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if created.AppID() != "sonarr" || created.HostPort != 8989 {
		t.Fatalf("got %+v", created)
	}

	list, _ := client.List(ctx)
	if len(list) != 3 {
		t.Fatalf("got %d containers after create, want 3 (2 fixtures + sonarr)", len(list))
	}

	if _, err := client.Create(ctx, spec); !errors.Is(err, dockerd.ErrAlreadyExists) {
		t.Fatalf("creating the same app twice: got %v, want ErrAlreadyExists", err)
	}

	if err := client.Remove(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx, created.ID); !errors.Is(err, dockerd.ErrNotFound) {
		t.Fatalf("starting a removed container: got %v, want ErrNotFound", err)
	}
}

func TestHTTPClient_ImageDigest(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()

	digest, err := client.ImageDigest(context.Background(), "pihole")
	if err != nil {
		t.Fatal(err)
	}
	if digest != "sha256:pihole-pulled-old" {
		t.Fatalf("got %q", digest)
	}

	if _, err := client.ImageDigest(context.Background(), "not-an-app"); err != dockerd.ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestHTTPClient_Attach(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()

	conn, err := client.Attach(context.Background(), "pihole")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("echo hi")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	got := string(buf[:n])
	if got == "" {
		t.Fatal("got no bytes back from the attached session")
	}
}

func state(list []dockerd.Container, id string) string {
	for _, c := range list {
		if c.ID == id {
			return c.State
		}
	}
	return ""
}

// A refused create travels back through the loopback hop as a typed
// refusal, and the mock daemon behind the Server never sees it.
func TestHTTPClient_CreateRefusals(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()
	ctx := context.Background()
	good := dockerd.ContainerSpec{
		AppID: "myapp", Image: "ghcr.io/x/y:1", MemoryLimitBytes: 1 << 20, HostNetwork: true, Root: "/media/hdd/dreamhub-apps",
		Mounts: []dockerd.Mount{{HostDir: "/media/hdd/dreamhub-apps/myapp", ContainerDir: "/config"}},
	}
	for name, mutate := range map[string]func(*dockerd.ContainerSpec){
		"host root":        func(s *dockerd.ContainerSpec) { s.Mounts[0].HostDir = "/" },
		"etc":              func(s *dockerd.ContainerSpec) { s.Mounts[0].HostDir = "/etc" },
		"docker socket":    func(s *dockerd.ContainerSpec) { s.Mounts[0].HostDir = "/var/run/docker.sock" },
		"dot dot":          func(s *dockerd.ContainerSpec) { s.Mounts[0].HostDir = "/media/hdd/../../etc" },
		"another app":      func(s *dockerd.ContainerSpec) { s.Mounts[0].HostDir = "/media/hdd/dreamhub-apps/pihole" },
		"unconfirmed priv": func(s *dockerd.ContainerSpec) { s.Elevated.Privileged = true },
	} {
		spec := good
		spec.Mounts = append([]dockerd.Mount{}, good.Mounts...)
		mutate(&spec)
		_, err := client.Create(ctx, spec)
		se, ok := dockerd.IsRefusal(err)
		if !ok || se.Code == "" || se.Message == "" {
			t.Errorf("%s: got %v, want a typed refusal", name, err)
		}
	}
	list, _ := client.List(ctx)
	if len(list) != 2 {
		t.Fatalf("a refused create changed the containers: %d", len(list))
	}
	if _, err := client.Create(ctx, good); err != nil {
		t.Fatalf("the good spec: %v", err)
	}
}

// The create surface takes the fixed shape only: a field it doesn't know
// (a raw HostConfig, a privileged flag outside "elevated") is refused, not
// ignored.
func TestServer_CreateRejectsUnknownFields(t *testing.T) {
	srv := httptest.NewServer(dockerd.Server{Client: mock.New()}.Handler())
	defer srv.Close()
	for _, body := range []string{
		`{"appId":"x","image":"x/y","memoryLimitBytes":1,"privileged":true}`,
		`{"appId":"x","image":"x/y","memoryLimitBytes":1,"hostConfig":{"Privileged":true}}`,
		`{"appId":"x","image":"x/y","memoryLimitBytes":1,"binds":["/:/host"]}`,
	} {
		resp, err := http.Post(srv.URL+"/containers", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", body, resp.StatusCode)
		}
	}
}

func TestHTTPClient_ImagePrune(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()
	ctx := context.Background()

	imgs, err := client.UnusedImages(ctx)
	if err != nil || len(imgs) != 3 {
		t.Fatalf("got %+v, %v", imgs, err)
	}
	res, err := client.PruneImages(ctx)
	if err != nil || res.Removed != 3 || res.ReclaimedBytes == 0 {
		t.Fatalf("got %+v, %v", res, err)
	}
	if imgs, _ = client.UnusedImages(ctx); len(imgs) != 0 {
		t.Fatalf("still %d unused images after the prune", len(imgs))
	}
}

// Prune takes no parameters at all: a body or a query string (a filter, a
// reference to one image) is refused instead of being ignored or passed on.
func TestServer_PruneTakesNoParameters(t *testing.T) {
	c := mock.New()
	srv := httptest.NewServer(dockerd.Server{Client: c}.Handler())
	defer srv.Close()
	for name, req := range map[string]func() (*http.Response, error){
		"body": func() (*http.Response, error) {
			return http.Post(srv.URL+"/images/prune", "application/json", strings.NewReader(`{"all":true}`))
		},
		"query": func() (*http.Response, error) { return http.Post(srv.URL+"/images/prune?filters=%7B%7D", "", nil) },
	} {
		resp, err := req()
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
	if imgs, _ := c.UnusedImages(context.Background()); len(imgs) != 3 {
		t.Fatalf("a refused request removed images: %d left", len(imgs))
	}
	// And no other image route exists (no delete of a chosen image, no raw API).
	for _, r := range []struct{ method, path string }{{"DELETE", "/images/sha256:aaaa1111"}, {"GET", "/images/json"}, {"POST", "/images/sha256:aaaa1111/tag"}} {
		req, _ := http.NewRequest(r.method, srv.URL+r.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status %d, want 404/405", r.method, r.path, resp.StatusCode)
		}
	}
}

// Keeping the previous image, seen through the helper: the helper builds the
// rollback name itself; DreamHub's side can name an app at most.
func TestHTTPClient_KeepAndRestoreAnImage(t *testing.T) {
	client, closeFn := newTestServer(t)
	defer closeFn()
	ctx := context.Background()

	id, err := client.KeepImage(ctx, "pihole")
	if err != nil || id == "" {
		t.Fatalf("KeepImage = %q, %v", id, err)
	}
	kept, err := client.KeptImages(ctx)
	if err != nil || len(kept) != 1 || kept[0].AppID != "pihole" || kept[0].ImageID != id {
		t.Fatalf("kept = %+v, %v", kept, err)
	}
	if err := client.Remove(ctx, "pihole"); err != nil {
		t.Fatal(err)
	}
	c, err := client.Create(ctx, dockerd.ContainerSpec{AppID: "pihole", Image: "pihole/pihole:latest", HostNetwork: true, HostPort: 80, ContainerPort: 80, MemoryLimitBytes: 1 << 28, FromKept: true})
	if err != nil || c.AppID() != "pihole" {
		t.Fatalf("restore: %+v, %v", c, err)
	}
	if d, _ := client.ImageDigest(ctx, "pihole"); d != "sha256:pihole-pulled-old" {
		t.Errorf("the restored container runs the kept image: digest %q", d)
	}
	if res, err := client.DropKeptImages(ctx, "pihole"); err != nil || res.Removed != 1 {
		t.Fatalf("drop = %+v, %v", res, err)
	}
	if kept, _ := client.KeptImages(ctx); len(kept) != 0 {
		t.Errorf("still kept: %+v", kept)
	}
	// Restoring when nothing is kept says so (410 on the wire).
	_ = client.Remove(ctx, "pihole")
	if _, err := client.Create(ctx, dockerd.ContainerSpec{AppID: "pihole", Image: "pihole/pihole:latest", HostNetwork: true, HostPort: 80, ContainerPort: 80, MemoryLimitBytes: 1 << 28, FromKept: true}); !errors.Is(err, dockerd.ErrNoKeptImage) {
		t.Errorf("nothing kept: %v", err)
	}
	if _, err := client.KeepImage(ctx, "nope"); !errors.Is(err, dockerd.ErrNotFound) {
		t.Errorf("unknown container: %v", err)
	}
}

// Dropping takes an app id and nothing else.
func TestServer_DropKeptTakesOnlyAnAppID(t *testing.T) {
	srv := httptest.NewServer(dockerd.Server{Client: mock.New()}.Handler())
	defer srv.Close()
	for path, want := range map[string]int{
		"/images/kept/drop":                         http.StatusOK,
		"/images/kept/drop?app=pihole":              http.StatusOK,
		"/images/kept/drop?app=Bad%20ID":            http.StatusBadRequest,
		"/images/kept/drop?image=sha256:aaaa1111":   http.StatusBadRequest,
		"/images/kept/drop?app=pihole&tag=whatever": http.StatusBadRequest,
	} {
		resp, err := http.Post(srv.URL+path, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("POST %s: %d, want %d", path, resp.StatusCode, want)
		}
	}
}
