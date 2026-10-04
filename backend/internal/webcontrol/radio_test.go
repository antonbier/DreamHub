package webcontrol

import "testing"

func TestIsRadioService(t *testing.T) {
	cases := map[string]bool{
		"1:0:2:6F37:44D:1:C00000:0:0:0:":                    true,  // radio
		"1:0:A:2F1C:3F3:1:C00000:0:0:0:":                    true,  // advanced codec radio
		"4097:0:2:0:0:0:0:0:0:0:http%3a//x/r.mp3:Web Radio": true,  // web radio stream
		"1:0:19:2EE3:441:1:C00000:0:0:0:":                   false, // HD TV
		"1:0:1:6DCA:44D:1:C00000:0:0:0:":                    false, // SD TV
		"1:64:1:0:0:0:0:0:0:0::— Musik —":                   false, // marker
		RadioRoot:                                           false, // a bouquet
		"1:0:0:0:0:0:0:0:0:0:/media/x.ts":                   false, // a recording
		"garbage":                                           false,
	}
	for ref, want := range cases {
		if got := IsRadioService(ref); got != want {
			t.Errorf("IsRadioService(%q) = %v, want %v", ref, got, want)
		}
	}
}

func TestIsMarker(t *testing.T) {
	if !IsMarker("1:64:1:0:0:0:0:0:0:0::— Musik —") {
		t.Error("marker not recognized")
	}
	if IsMarker("1:0:2:6F37:44D:1:C00000:0:0:0:") {
		t.Error("station taken for a marker")
	}
}
