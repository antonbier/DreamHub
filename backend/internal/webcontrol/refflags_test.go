package webcontrol

import "testing"

// enigma2 writes a reference's type and flags in decimal (eServiceReference::
// toString), the other fields in hex.
func TestIsMarker_FlagsAreDecimal(t *testing.T) {
	for ref, want := range map[string]bool{
		"1:64:0:0:0:0:0:0:0:0:":           true, // a heading
		"1:0:19:283D:3FB:1:C00000:0:0:0:": false,
		"1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"userbouquet.x.tv\" ORDER BY bouquet": false,
		// Netcast: flags 256 (sortable) must not look like the marker bit
		"8193:256:0:73:1:66:515476:0:0:0:netcastserver_lgat%3a//h/p.m3u8:X": false,
		"1:320:0:0:0:0:0:0:0:0:": true, // 320 = sortable + marker
	} {
		if got := IsMarker(ref); got != want {
			t.Errorf("IsMarker(%q) = %v, want %v", ref, got, want)
		}
	}
	if !IsRadioService("1:0:2:1:1:1:C00000:0:0:0:") || IsRadioService("1:7:2:0:0:0:0:0:0:0:FROM BOUQUET x") {
		t.Error("radio detection changed")
	}
}
