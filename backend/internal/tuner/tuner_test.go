package tuner

import (
	"reflect"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Real service references (Astra 19.2°E, namespace C00000): RTL and VOX
// share one transponder (TSID 0x441), zdf_neo HD is on another (0x3F3).
const (
	rtl    = "1:0:19:2EE3:441:1:C00000:0:0:0:"
	vox    = "1:0:19:2F1C:441:1:C00000:0:0:0:"
	zdfneo = "1:0:19:2B7A:3F3:1:C00000:0:0:0:"
	arte   = "1:0:19:283D:3FB:1:C00000:0:0:0:"
	cable  = "1:0:19:6D66:437:85:FFFF0000:0:0:0:"
	dvbt   = "1:0:19:2BD:3F0:1:EEEE0000:0:0:0:"
	movie  = "1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/x.ts"
	sat4   = "1:0:19:2E9B:3F8:1:C00000:0:0:0:" // a fourth Astra transponder
)

func box(models ...string) []Frontend {
	info := &webcontrol.DeviceInfo{}
	for _, m := range models {
		info.Frontends = append(info.Frontends, webcontrol.Frontend{Name: "Tuner", Model: m})
	}
	return Frontends(info)
}

// Example boxes. Only the reference box is real (ARCHITECTURE.md); the
// others model common Dreambox setups — real deviceinfo samples from those
// models should replace them when available.
var (
	referenceBox = box(" Si2169D (DVB-S2 / DVB-C / DVB-T2)")
	tripleSat    = box("DVB-S2", "DVB-S2", "DVB-S2")                           // e.g. DM7080 with three satellite tuners
	fbcBox       = box("FBC DVB-S2", "FBC DVB-S2", "FBC DVB-S2", "FBC DVB-S2", // one FBC input shows up
		"FBC DVB-S2", "FBC DVB-S2", "FBC DVB-S2", "FBC DVB-S2") // as 8 frontends
	mixedSatCable = box("DVB-S2", "DVB-C")
)

func b(ref string, begin, end int64) Booking {
	return Booking{ServiceReference: ref, Begin: begin, End: end}
}

func TestConflicts(t *testing.T) {
	cases := []struct {
		name      string
		frontends []Frontend
		existing  []Booking
		candidate Booking
		want      []int
	}{
		{"single tuner, other transponder overlaps", referenceBox,
			[]Booking{b(zdfneo, 100, 200)}, b(rtl, 150, 250), []int{0}},
		{"single tuner, same transponder records both", referenceBox,
			[]Booking{b(vox, 100, 200)}, b(rtl, 150, 250), nil},
		{"single tuner, back to back is fine", referenceBox,
			[]Booking{b(zdfneo, 100, 200)}, b(rtl, 200, 300), nil},
		{"single tuner, playback needs no tuner", referenceBox,
			[]Booking{b(movie, 100, 200)}, b(rtl, 100, 200), nil},
		{"three tuners, three transponders fit", tripleSat,
			[]Booking{b(zdfneo, 100, 200), b(arte, 100, 200)}, b(rtl, 100, 200), nil},
		{"three tuners, a fourth transponder doesn't", tripleSat,
			[]Booking{b(zdfneo, 100, 200), b(arte, 100, 200), b(sat4, 100, 200)}, b(rtl, 100, 200), []int{0, 1, 2}},
		{"three tuners, only the overlapping instant counts", tripleSat,
			// arte ends before zdfneo starts: never more than 3 at once.
			[]Booking{b(arte, 100, 150), b(zdfneo, 150, 250), b(sat4, 100, 250)}, b(rtl, 100, 250), nil},
		{"FBC, 8 frontends take 4 transponders", fbcBox,
			[]Booking{b(zdfneo, 0, 10), b(arte, 0, 10), b(vox, 0, 10)}, b(rtl, 0, 10), nil},
		{"satellite tuner can't take a cable channel", mixedSatCable,
			[]Booking{b(cable, 100, 200)}, b(rtl, 100, 200), nil},
		{"second cable channel has no free cable tuner", mixedSatCable,
			[]Booking{b(cable, 100, 200)}, b("1:0:19:6D67:438:85:FFFF0000:0:0:0:", 100, 200), []int{0}},
		{"terrestrial channel on a sat+cable box never fits", mixedSatCable,
			nil, b(dvbt, 100, 200), nil}, // nothing to clash with: the box itself will refuse
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Conflicts(c.frontends, c.existing, c.candidate); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Conflicts() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseService(t *testing.T) {
	s, ok := ParseService(rtl)
	if !ok || s.SID != 0x2EE3 || s.Transponder.TSID != 0x441 || s.Delivery() != Satellite {
		t.Errorf("rtl: %+v %v", s, ok)
	}
	if s, _ := ParseService(cable); s.Delivery() != Cable {
		t.Errorf("cable: %v", s.Delivery())
	}
	if s, _ := ParseService(dvbt); s.Delivery() != Terrestrial {
		t.Errorf("dvbt: %v", s.Delivery())
	}
	for _, ref := range []string{movie, "", "4097:0:1:0:0:0:0:0:0:0:http%3a//x", "1:0:19:zz:1:1:C00000:0:0:0:"} {
		if _, ok := ParseService(ref); ok {
			t.Errorf("%q should not count as a tuner service", ref)
		}
	}
}

func TestSameService(t *testing.T) {
	// Flags and service type differ, the channel is the same — the box's
	// duplicate check compares only SID/TSID/ONID/namespace.
	if !SameService("1:0:19:2EE3:441:1:C00000:0:0:0:", "1:0:1:2EE3:441:1:C00000:0:0:0:") {
		t.Error("same channel, different service type, should match")
	}
	if SameService(rtl, vox) {
		t.Error("RTL and VOX are different channels")
	}
}

func TestFrontends(t *testing.T) {
	if got := Frontends(nil); len(got) != 1 || !got[0].can(Cable) {
		t.Errorf("no device info should mean one universal tuner, got %+v", got)
	}
	if f := referenceBox[0]; !f.can(Satellite) || !f.can(Cable) || !f.can(Terrestrial) {
		t.Errorf("the reference combo tuner should receive everything: %+v", f)
	}
	if f := mixedSatCable[0]; f.can(Cable) {
		t.Errorf("a DVB-S2 tuner can't receive cable: %+v", f)
	}
}
