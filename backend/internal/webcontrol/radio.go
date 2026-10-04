package webcontrol

import (
	"strconv"
	"strings"
)

// RadioRoot is the reference of the box's radio bouquet list: passed to
// Services it lists the radio bouquets, the way /web/getservices without
// sRef lists the TV ones. This is Enigma2's own convention (the TV root is
// the same with service type 1 and "bouquets.tv"); confirmed on the
// reference box 2026-10-04 (fixture getservices_radio_root.xml).
const RadioRoot = `1:7:2:0:0:0:0:0:0:0:FROM BOUQUET "bouquets.radio" ORDER BY bouquet`

// RadioAll lists every radio service the box knows, by name — every
// transponder's, so a station can appear more than once, and data
// services named "." are among them (confirmed live 2026-10-04: 664
// entries in 0.06 s, epgnownext over it in 0.25 s). RadioHub offers it
// as "All radio stations", the only list there is when the radio
// favourites are empty, as on the reference box.
const RadioAll = `1:7:2:0:0:0:0:0:0:0:(type == 2) || (type == 10) ORDER BY name`

// serviceFields splits a service reference into its numeric fields
// ("1:0:2:6F37:..." → 1, 0, 0x2, 0x6F37, …), hex as Enigma2 writes them.
// ok is false for anything that isn't one.
func serviceFields(ref string) (fields []uint64, ok bool) {
	parts := strings.Split(ref, ":")
	if len(parts) < 10 {
		return nil, false
	}
	for _, p := range parts[:10] {
		v, err := strconv.ParseUint(p, 16, 32)
		if err != nil {
			return nil, false
		}
		fields = append(fields, v)
	}
	return fields, true
}

// Flags in a reference's second field.
const (
	flagDirectory = 0x01 // a bouquet or another list, not a service
	flagMarker    = 0x40 // a heading inside a bouquet
)

// IsRadioService reports whether ref is a radio station: DVB service type
// 0x02 (digital radio) or 0x0A (advanced codec radio) in the third field,
// and neither a list (the radio root and bouquets carry type 2 as well)
// nor a heading.
func IsRadioService(ref string) bool {
	f, ok := serviceFields(ref)
	return ok && f[1]&(flagDirectory|flagMarker) == 0 && (f[2] == 0x02 || f[2] == 0x0A)
}

// IsMarker reports whether ref is a bouquet's section heading ("1:64:…"):
// a label between channels, not something to play or record.
func IsMarker(ref string) bool {
	f, ok := serviceFields(ref)
	return ok && f[1]&flagMarker != 0
}
