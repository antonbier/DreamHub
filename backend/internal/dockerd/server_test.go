package dockerd_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
		Env:           map[string]string{"TZ": "Europe/Rome"},
		HostPort:      8989, ContainerPort: 8989,
		HostDir:       "/media/hdd/dreamhub-apps/sonarr", ContainerDir: "/config",
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
