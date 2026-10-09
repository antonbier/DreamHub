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
	ok, err := st.ReserveAIUsage(ctx, now().UTC().Format("2006-01-02"), v.Int(connector.KeyAIDailyLimit))
	switch {
	case err != nil:
		return "internal"
	case !ok:
		return "ai_budget_exceeded"
	}
	return ""
}
