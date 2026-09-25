package hubs

import (
	"context"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// reserveAICall books one call against the shared daily AI budget
// (ai.dailyLimit, one row per UTC date) before the call is made — a slow or
// failed call still counts, so a retry-happy client can't exceed the cap
// (same rule as DiscoveryHub). It returns an error code, or "" when the
// call may go ahead.
func reserveAICall(ctx context.Context, st *store.Store, v settings.Values, now func() time.Time) string {
	today := now().UTC().Format("2006-01-02")
	used, err := st.AIUsageToday(ctx, today)
	if err != nil {
		return "internal"
	}
	if int64(used) >= v.Int(connector.KeyAIDailyLimit) {
		return "ai_budget_exceeded"
	}
	if _, err := st.IncrementAIUsage(ctx, today); err != nil {
		return "internal"
	}
	return ""
}
