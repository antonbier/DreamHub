package hubs

import (
	"strings"
	"testing"
)

func TestLogoKeyNeverHoldsCredentials(t *testing.T) {
	a := "4097:0:1:1:0:0:0:0:0:0:rtsp%3a//bob:s3cret@10.0.0.5/live/1:One"
	b := "4097:0:1:1:0:0:0:0:0:0:rtsp%3a//bob:s3cret@10.0.0.5/live/2:Two"
	ka, kb := logoKey(a), logoKey(b)
	if ka == "" || ka == kb {
		t.Fatalf("IPTV channels with equal ten fields need distinct keys: %q %q", ka, kb)
	}
	for _, k := range []string{ka, kb} {
		if strings.Contains(k, "bob") || strings.Contains(k, "s3cret") || strings.ContainsAny(k, "/@:%") {
			t.Fatalf("key leaks the reference tail: %q", k)
		}
	}
	// Changing the password keeps the key, so a chosen logo survives.
	if logoKey(strings.ReplaceAll(a, "s3cret", "other")) != ka {
		t.Fatal("key depends on the password")
	}
	// A plain broadcast reference keeps its picon-style key.
	if got := logoKey("1:0:19:2EE3:441:1:C00000:0:0:0:"); got != "1_0_19_2EE3_441_1_C00000_0_0_0" {
		t.Fatalf("broadcast key: %q", got)
	}
}
