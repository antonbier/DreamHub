package httpx

import "testing"

func TestHostAllowed(t *testing.T) {
	allow := []string{"dreamhub.home.arpa"}
	for host, want := range map[string]bool{
		"192.168.1.43":            true,
		"192.168.1.43:8080":       true,
		"[fdb2:f390::1]:8080":     true,
		"localhost:8080":          true,
		"dreambox.local":          true,
		"DreamBox.Local.":         true,
		"dreamhub.home.arpa:8080": true,
		"evil.example.com":        false,
		"192.168.1.43.nip.io":     false, // classic rebinding-style name
		"":                        false,
	} {
		if got := HostAllowed(host, allow); got != want {
			t.Errorf("HostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
}
