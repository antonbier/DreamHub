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

import "context"

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
	Recommend(ctx context.Context, apiKey, model string, items []EPGItem) ([]Recommendation, error)
}
