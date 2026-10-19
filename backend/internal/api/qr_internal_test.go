package api

import (
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
