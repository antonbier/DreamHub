package systemd

import (
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
