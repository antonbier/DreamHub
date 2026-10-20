package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReachableHost(t *testing.T) {
	for _, host := range []string{"192.168.1.43:8765", "dm920:8765", "dreambox.local"} {
		if got := reachableHost(host); got != host {
			t.Errorf("reachableHost(%q) = %q, want unchanged", host, got)
		}
	}
	for _, host := range []string{"127.0.0.1:8765", "localhost:8765", "[::1]:8765"} {
		got := reachableHost(host)
		if got == host {
			t.Skip("no LAN address on this machine")
		}
		if !strings.HasSuffix(got, ":8765") || strings.HasPrefix(got, "127.") {
			t.Errorf("reachableHost(%q) = %q, want the LAN address with the same port", host, got)
		}
	}
}

func TestOriginURL_NeverLoopback(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/qr.png", nil)
	r.Host = "127.0.0.1:8765"
	got := originURL(r)
	if strings.Contains(got, "127.0.0.1") && reachableHost("127.0.0.1:8765") != "127.0.0.1:8765" {
		t.Fatalf("the QR code must not point at the box's own loopback: %s", got)
	}
	if !strings.HasPrefix(got, "http://") || !strings.HasSuffix(got, ":8765") {
		t.Fatalf("address: %s", got)
	}
	r.Host = "dm920:8765"
	if got := originURL(r); got != "http://dm920:8765" {
		t.Fatalf("a host name is kept: %s", got)
	}
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Host = "box.example.org"
	if got := originURL(r); got != "https://box.example.org" {
		t.Fatalf("behind a proxy: %s", got)
	}
}
