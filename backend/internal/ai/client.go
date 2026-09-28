// Package ai is the AI-provider connector's actual client interface
// (connector.OpenAI owns only the settings/reachability check —
// internal/connector/builtin.go's comment says the client interface itself
// "comes with DiscoveryHub"). OpenAI is the first (and today only)
// provider; PRINCIPLES.md calls for this to stay swappable.
//
// Grounding, not generation: the model is given a numbered list of real EPG
// events and asked to pick indices, not to invent broadcasts. DiscoveryHub
// then builds each Recommendation from DreamHub's own EPGItem data at that
// index plus the model's reason text — never from fields the model could
// have hallucinated (a wrong service reference here would create a real
// timer for the wrong program).
package ai

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/naming"
)

// EPGItem is one candidate broadcast offered to the AI, trimmed for token
// budget (ROADMAP.md: "~90k tokens/day for 30 channels is cents-level").
type EPGItem struct {
	Title            string
	Description      string
	ServiceReference string
	ServiceName      string
	Start            int64 // Unix seconds, UTC
	Duration         int64 // seconds
}

// Taste is what the user already records, plans and follows — titles only,
// so the model can lean its picks toward the user's taste instead of
// "generally good TV". Empty means no profile (the user switched it off,
// or nothing is known yet); every list is capped by DiscoveryHub.
type Taste struct {
	Recordings []string `json:"recordings,omitempty"` // recent recordings' titles
	Series     []string `json:"series,omitempty"`     // series followed in SerienRecorder
	Timers     []string `json:"timers,omitempty"`     // upcoming timers' titles
	AutoTimers []string `json:"autoTimers,omitempty"` // AutoTimer search terms
}

// Empty reports whether t carries no signal at all.
func (t Taste) Empty() bool {
	return len(t.Recordings) == 0 && len(t.Series) == 0 && len(t.Timers) == 0 && len(t.AutoTimers) == 0
}

// Recommendation is one AI pick, built from an EPGItem plus the model's
// stated reason.
type Recommendation struct {
	Title            string `json:"title"`
	ServiceReference string `json:"serviceReference"`
	ServiceName      string `json:"serviceName"`
	Start            int64  `json:"start"`
	Duration         int64  `json:"duration"`
	Reason           string `json:"reason"`
}

// Client is anything that can turn a candidate list into recommendations —
// implemented by both the real HTTP client below and the mock in ./mock,
// per PRINCIPLES.md's "Mock-first development". model empty means "the
// provider's default".
type Client interface {
	// taste may be empty; the picks are still grounded in items only.
	Recommend(ctx context.Context, apiKey, model string, items []EPGItem, taste Taste) ([]Recommendation, error)
	// SuggestName reads a recording's own metadata and returns structured
	// fields (title, year, season, episode, episode title) — never a file
	// name: MediaHub builds that itself (internal/naming), and rejects
	// fields that don't pass naming.Fields.Valid.
	SuggestName(ctx context.Context, apiKey, model string, in NameInput) (naming.Fields, error)
}

// NameInput is what the AI sees of a recording: its real metadata only.
type NameInput struct {
	FileName    string `json:"fileName"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Channel     string `json:"channel,omitempty"`
	Folder      string `json:"folder,omitempty"` // path below the recording folder
}
