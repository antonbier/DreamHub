package meminfo

import (
	"context"
	"testing"
)

func TestExec_ReadsRealProcMeminfo(t *testing.T) {
	// This sandbox is Linux, so /proc/meminfo is real — proves the
	// parser against actual kernel output, not just a fixture.
	info, err := Exec{}.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalBytes <= 0 || info.AvailableBytes <= 0 {
		t.Fatalf("got %+v, want positive totals", info)
	}
	if info.AvailableBytes > info.TotalBytes {
		t.Fatalf("available (%d) exceeds total (%d)", info.AvailableBytes, info.TotalBytes)
	}
}
