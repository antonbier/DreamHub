// Package mock implements ai.Client without calling any real AI provider,
// so DiscoveryHub is buildable, testable and demoable without an API key or
// network access or cost — see PRINCIPLES.md, "Mock-first development".
package mock

import (
	"context"
	"github.com/antonbier/DreamHub/backend/internal/naming"

	"github.com/antonbier/DreamHub/backend/internal/ai"
)

// maxPicks mirrors the real client's cap, so the mock exercises the same
// "at most a handful of picks" shape callers must handle.
const maxPicks = 3

// Client is a deterministic ai.Client: it "recommends" the first few
// candidates it's given, with a canned reason, regardless of apiKey/model.
type Client struct {
	// LastTaste is the taste profile of the most recent Recommend call,
	// for tests to assert on.
	LastTaste ai.Taste
}

var _ ai.Client = (*Client)(nil)

// New returns a mock client.
func New() *Client { return &Client{} }

func (c *Client) Recommend(ctx context.Context, apiKey, model string, items []ai.EPGItem, taste ai.Taste) ([]ai.Recommendation, error) {
	c.LastTaste = taste
	n := min(len(items), maxPicks)
	out := make([]ai.Recommendation, 0, n)
	reason := "Mock pick for local development (internal/ai/mock) — not a real AI recommendation."
	if !taste.Empty() {
		reason = "Mock pick with a taste profile (internal/ai/mock) — not a real AI recommendation."
	}
	for _, it := range items[:n] {
		out = append(out, ai.Recommendation{
			Title: it.Title, ServiceReference: it.ServiceReference, ServiceName: it.ServiceName,
			Start: it.Start, Duration: it.Duration, Reason: reason,
		})
	}
	return out, nil
}

// SuggestName derives the fields from the recording's own title, like the
// metadata source does — deterministic, free, no network.
func (*Client) SuggestName(ctx context.Context, apiKey, model string, in ai.NameInput) (naming.Fields, error) {
	return naming.FromMetadata(in.Title, in.Description, nil), nil
}

// ExplainPackage returns a canned, clearly-mock explanation — deterministic,
// free, no network.
func (*Client) ExplainPackage(ctx context.Context, apiKey, model, name, description string) (string, error) {
	return "Mock explanation for local development (internal/ai/mock) — not a real AI answer: " + name + ".", nil
}
