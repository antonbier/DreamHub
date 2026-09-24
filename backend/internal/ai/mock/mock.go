// Package mock implements ai.Client without calling any real AI provider,
// so DiscoveryHub is buildable, testable and demoable without an API key or
// network access or cost — see PRINCIPLES.md, "Mock-first development".
package mock

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/ai"
)

// maxPicks mirrors the real client's cap, so the mock exercises the same
// "at most a handful of picks" shape callers must handle.
const maxPicks = 3

// Client is a deterministic ai.Client: it "recommends" the first few
// candidates it's given, with a canned reason, regardless of apiKey/model.
type Client struct{}

var _ ai.Client = (*Client)(nil)

// New returns a mock client.
func New() *Client { return &Client{} }

func (*Client) Recommend(ctx context.Context, apiKey, model string, items []ai.EPGItem) ([]ai.Recommendation, error) {
	n := min(len(items), maxPicks)
	out := make([]ai.Recommendation, 0, n)
	for _, it := range items[:n] {
		out = append(out, ai.Recommendation{
			Title: it.Title, ServiceReference: it.ServiceReference, ServiceName: it.ServiceName,
			Start: it.Start, Duration: it.Duration,
			Reason: "Mock pick for local development (internal/ai/mock) — not a real AI recommendation.",
		})
	}
	return out, nil
}
