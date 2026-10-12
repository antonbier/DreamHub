package dockerhub_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
)

func fakeHub(t *testing.T) (*dockerhub.HTTPClient, func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/search/repositories/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":1,"results":[{"repo_name":"pihole/pihole","short_description":"ad blocking","star_count":2900}]}`))
	})
	calls := 0
	mux.HandleFunc("GET /v2/repositories/pihole/pihole/tags/latest", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[{"architecture":"amd64"},{"architecture":"arm64"},{"architecture":"arm","variant":"v7"}]}`))
	})
	mux.HandleFunc("GET /v2/repositories/nope/nope/tags/latest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"object not found"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(func() { t.Logf("manifest fetches: %d", calls) })
	return &dockerhub.HTTPClient{BaseURL: srv.URL}, srv.Close
}

func TestHTTPClient_Search(t *testing.T) {
	client, closeFn := fakeHub(t)
	defer closeFn()

	results, err := client.Search(context.Background(), "pihole")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "pihole/pihole" || results[0].Stars != 2900 {
		t.Fatalf("got %+v", results)
	}
}

func TestHTTPClient_Manifest(t *testing.T) {
	client, closeFn := fakeHub(t)
	defer closeFn()
	ctx := context.Background()

	arches, err := client.Manifest(ctx, "pihole/pihole", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if !dockerhub.Supports(arches, dockerhub.Arch{Architecture: "arm64"}) {
		t.Fatalf("got %+v, want arm64 support", arches)
	}
	if !dockerhub.Supports(arches, dockerhub.Arch{Architecture: "arm", Variant: "v7"}) {
		t.Fatalf("got %+v, want armhf (arm/v7) support", arches)
	}
}

func TestHTTPClient_ManifestNotFound(t *testing.T) {
	client, closeFn := fakeHub(t)
	defer closeFn()

	if _, err := client.Manifest(context.Background(), "nope/nope", "latest"); !errors.Is(err, dockerhub.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestHTTPClient_ManifestCached(t *testing.T) {
	client, closeFn := fakeHub(t)
	defer closeFn()
	ctx := context.Background()

	if _, err := client.Manifest(ctx, "pihole/pihole", "latest"); err != nil {
		t.Fatal(err)
	}
	// A second call within manifestTTL must be served from cache, not a
	// second HTTP round trip — proven by pointing BaseURL at a server
	// that would error if hit again.
	client.BaseURL = "http://127.0.0.1:1"
	arches, err := client.Manifest(ctx, "pihole/pihole", "latest")
	if err != nil {
		t.Fatalf("expected the cached result, got an error: %v", err)
	}
	if !dockerhub.Supports(arches, dockerhub.Arch{Architecture: "arm64"}) {
		t.Fatalf("got %+v from cache, want the original result", arches)
	}
}

func TestHTTPClient_Unreachable(t *testing.T) {
	client := &dockerhub.HTTPClient{BaseURL: "http://127.0.0.1:1"}
	if _, err := client.Search(context.Background(), "pihole"); !errors.Is(err, dockerhub.ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}
