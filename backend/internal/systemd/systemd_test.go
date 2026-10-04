package systemd

import (
	"os"
	"testing"
	"time"
)

// Shaped like `systemctl show --property=…` for three units, one unknown.
const showOutput = `Id=enigma2.service
Description=enigma2
LoadState=loaded
ActiveState=active
SubState=running
ActiveEnterTimestampMonotonic=12500000
MainPID=812
MemoryCurrent=[not set]
NRestarts=2

Id=smbd.service
Description=Samba SMB Daemon
LoadState=loaded
ActiveState=failed
SubState=failed
ActiveEnterTimestampMonotonic=0
MainPID=0
MemoryCurrent=18446744073709551615
NRestarts=0

Id=nope.service
Description=nope.service
LoadState=not-found
ActiveState=inactive
SubState=dead
ActiveEnterTimestampMonotonic=0
MainPID=0
MemoryCurrent=[not set]
NRestarts=0
`

func TestParseShow(t *testing.T) {
	boot := time.Date(2026, 10, 4, 6, 0, 0, 0, time.UTC)
	units := ParseShow([]byte(showOutput), boot)
	if len(units) != 2 {
		t.Fatalf("the unknown unit must be left out, got %+v", units)
	}
	e2 := units[0]
	if e2.Name != "enigma2.service" || e2.Active != "active" || e2.Sub != "running" || e2.MainPID != 812 || e2.Restarts != 2 || e2.Memory != 0 {
		t.Errorf("enigma2: %+v", e2)
	}
	if want := boot.Add(12500 * time.Millisecond).Truncate(time.Second); !e2.Since.Equal(want) {
		t.Errorf("since = %v, want %v", e2.Since, want)
	}
	smb := units[1]
	if smb.Active != "failed" || !smb.Since.IsZero() || smb.Memory != 0 {
		t.Errorf("samba: %+v", smb)
	}
}

func TestParseShow_NoBootTimeLeavesSinceUnset(t *testing.T) {
	if u := ParseShow([]byte(showOutput), time.Time{}); !u[0].Since.IsZero() {
		t.Errorf("got %v", u[0].Since)
	}
}

// The reference box's own answer (systemd 232, captured by the self-test
// 2026-10-04): no NRestarts, MemoryCurrent "unset" as 2^64-1, properties
// in its own order, unknown units left out.
func TestParseShow_ReferenceBox232(t *testing.T) {
	b, err := os.ReadFile("../../testdata/fixtures/systemctl_show_232.txt")
	if err != nil {
		t.Fatal(err)
	}
	units := ParseShow(b, time.Date(2026, 10, 4, 16, 50, 0, 0, time.UTC))
	if len(units) != 8 || units[0].Name != "enigma2.service" || units[0].MainPID != 1842 || units[0].Active != "active" {
		t.Fatalf("got %+v", units)
	}
	for _, u := range units {
		if u.Memory != 0 || u.Restarts != 0 {
			t.Errorf("%s: memory/restarts not reported on 232, got %+v", u.Name, u)
		}
		if u.Active == "active" && u.Since.IsZero() {
			t.Errorf("%s: since missing", u.Name)
		}
	}
}
