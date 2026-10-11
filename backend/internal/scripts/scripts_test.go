package scripts

import (
	"context"
	"strings"
	"testing"
)

func TestExec_RunsAndCapturesOutput(t *testing.T) {
	out, code, err := Exec{}.Run(context.Background(), "#!/bin/sh\necho hello\n")
	if err != nil || code != 0 || out != "hello\n" {
		t.Fatalf("out=%q code=%d err=%v", out, code, err)
	}
}

func TestExec_ReportsANonZeroExitCode(t *testing.T) {
	_, code, err := Exec{}.Run(context.Background(), "#!/bin/sh\nexit 7\n")
	if err != nil || code != 7 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestExec_TruncatesHugeOutput(t *testing.T) {
	// Prints more than MaxOutput bytes.
	out, code, err := Exec{}.Run(context.Background(), "#!/bin/sh\nyes | head -c 300000\n")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if len(out) > MaxOutput+100 || !strings.Contains(out, "truncated") {
		t.Fatalf("expected truncated output, got %d bytes", len(out))
	}
}
