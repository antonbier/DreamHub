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

	out, err := client.Logs(context.Background(), "adguard", 50)
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
	// Nothing beyond list/start/stop/logs/exec is reachable — in
	// particular no create/run, the hard denial the plan requires.
	srv := httptest.NewServer(dockerd.Server{Client: mock.New()}.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/containers/pihole/create", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST .../create: got status %d, want 404 (no such route)", resp.StatusCode)
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
