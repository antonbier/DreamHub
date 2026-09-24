package store

import (
	"context"
	"testing"
)

func TestAIUsage_IncrementsPerDateAndReadsBack(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()

	if n, err := s.AIUsageToday(ctx, "2026-09-24"); err != nil || n != 0 {
		t.Fatalf("expected 0 for a never-used date, got %d, %v", n, err)
	}

	n, err := s.IncrementAIUsage(ctx, "2026-09-24")
	if err != nil || n != 1 {
		t.Fatalf("expected 1 after the first increment, got %d, %v", n, err)
	}
	n, err = s.IncrementAIUsage(ctx, "2026-09-24")
	if err != nil || n != 2 {
		t.Fatalf("expected 2 after the second increment, got %d, %v", n, err)
	}

	// A different date must have its own independent count.
	if n, err := s.AIUsageToday(ctx, "2026-09-25"); err != nil || n != 0 {
		t.Fatalf("expected a different date to start at 0, got %d, %v", n, err)
	}
	if n, err := s.AIUsageToday(ctx, "2026-09-24"); err != nil || n != 2 {
		t.Fatalf("expected the original date's count to still be 2, got %d, %v", n, err)
	}
}
