package ai

import (
	"github.com/antonbier/DreamHub/backend/internal/naming"

	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrNotConfigured is returned when no API key was supplied.
var ErrNotConfigured = errors.New("ai: no API key configured")

// maxItems caps the candidate list sent to the model — a full day across
// 30 channels can be several hundred events; DiscoveryHub already narrows
// to a bouquet and a time window, this is a last-resort ceiling.
const maxItems = 300

// maxPicks caps how many recommendations a single response can return,
// regardless of what the model claims.
const maxPicks = 8

const defaultModel = "gpt-4o-mini"

const systemPrompt = `You are DreamHub's TV recommendation assistant. You will receive a JSON object with "candidates", a numbered list of upcoming broadcasts (today's EPG for the user's selected channels), and optionally "taste": what the user already records ("recordings"), follows as series ("series"), has planned ("timers") and searches for automatically ("autoTimers"). Recommend up to 8 candidates genuinely worth watching or recording. When a taste is given, favor candidates that fit it — same genres, topics, formats or people — but don't recommend the very shows the user already follows or has planned; find related things they might have missed. Favor variety over repeats of the same show. Respond with ONLY a JSON object of this exact shape: {"picks":[{"index":<integer index from the candidates>,"reason":"<one short sentence, same language as the titles; when a pick fits the taste, say which of the user's shows it relates to>"}]}. Only use indices that appear in the candidates. If nothing stands out, return {"picks":[]}.`

// maxTasteItems caps each taste list sent to the model.
const maxTasteItems = 40

// OpenAIClient calls OpenAI's chat completions API.
type OpenAIClient struct {
	HTTP    *http.Client
	BaseURL string // overridable in tests; defaults to the real API
}

// NewOpenAIClient returns an OpenAIClient with a generous timeout — a
// completion can take several seconds.
func NewOpenAIClient() *OpenAIClient {
	return &OpenAIClient{HTTP: &http.Client{Timeout: 30 * time.Second}, BaseURL: "https://api.openai.com"}
}

type promptItem struct {
	Index           int    `json:"index"`
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	Channel         string `json:"channel"`
	StartUTC        string `json:"startUtc"`
	DurationMinutes int64  `json:"durationMinutes"`
}

type pick struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

type picksResponse struct {
	Picks []pick `json:"picks"`
}

type chatRequest struct {
	Model          string             `json:"model"`
	Messages       []chatMessage      `json:"messages"`
	ResponseFormat chatResponseFormat `json:"response_format"`
	MaxTokens      int                `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenAIClient) Recommend(ctx context.Context, apiKey, model string, items []EPGItem, taste Taste) ([]Recommendation, error) {
	if apiKey == "" {
		return nil, ErrNotConfigured
	}
	if len(items) == 0 {
		return nil, nil
	}
	if len(items) > maxItems {
		items = items[:maxItems]
	}

	prompts := make([]promptItem, len(items))
	for i, it := range items {
		prompts[i] = promptItem{
			Index: i, Title: it.Title, Description: truncate(it.Description, 200), Channel: it.ServiceName,
			StartUTC: time.Unix(it.Start, 0).UTC().Format(time.RFC3339), DurationMinutes: it.Duration / 60,
		}
	}
	var tastePtr *Taste
	if !taste.Empty() {
		capped := Taste{
			Recordings: capList(taste.Recordings), Series: capList(taste.Series),
			Timers: capList(taste.Timers), AutoTimers: capList(taste.AutoTimers),
		}
		tastePtr = &capped
	}
	userContent, err := json.Marshal(struct {
		Candidates []promptItem `json:"candidates"`
		Taste      *Taste       `json:"taste,omitempty"`
	}{prompts, tastePtr})
	if err != nil {
		return nil, fmt.Errorf("ai: encoding candidate list: %w", err)
	}
	content, err := c.chat(ctx, apiKey, model, systemPrompt, string(userContent), 600)
	if err != nil {
		return nil, err
	}

	var picks picksResponse
	if err := json.Unmarshal([]byte(content), &picks); err != nil {
		return nil, fmt.Errorf("ai: parsing recommendation JSON: %w", err)
	}

	out := make([]Recommendation, 0, min(len(picks.Picks), maxPicks))
	for _, p := range picks.Picks {
		if len(out) >= maxPicks {
			break
		}
		if p.Index < 0 || p.Index >= len(items) {
			continue // grounding: never trust an out-of-range index
		}
		it := items[p.Index]
		out = append(out, Recommendation{
			Title: it.Title, ServiceReference: it.ServiceReference, ServiceName: it.ServiceName,
			Start: it.Start, Duration: it.Duration, Reason: p.Reason,
		})
	}
	return out, nil
}

// ErrInvalidAnswer means the model's answer didn't pass validation — it's
// dropped, never used half-way.
var ErrInvalidAnswer = errors.New("ai: the answer didn't pass validation")

const namePrompt = `You are DreamHub's recording-naming assistant. You receive one TV recording's real metadata as JSON (file name, broadcast title, description, channel, folder). Work out what it is and respond with ONLY a JSON object of this exact shape: {"title":"<series or film title>","year":<release year of a film, or 0>,"season":<season number or 0>,"episode":<episode number or 0>,"episodeTitle":"<episode title or empty>"}. Use the language and spelling of the metadata; don't translate titles. Only fill in what the metadata supports; use 0 or "" when unsure. For a series episode, leave "year" at 0.`

func (c *OpenAIClient) SuggestName(ctx context.Context, apiKey, model string, in NameInput) (naming.Fields, error) {
	if apiKey == "" {
		return naming.Fields{}, ErrNotConfigured
	}
	in.Description = truncate(in.Description, 600)
	userContent, err := json.Marshal(in)
	if err != nil {
		return naming.Fields{}, err
	}
	content, err := c.chat(ctx, apiKey, model, namePrompt, string(userContent), 200)
	if err != nil {
		return naming.Fields{}, err
	}
	var f naming.Fields
	if err := json.Unmarshal([]byte(content), &f); err != nil || !f.Valid() {
		return naming.Fields{}, ErrInvalidAnswer
	}
	return f, nil
}

// chat runs one JSON-mode chat completion and returns the message content.
func (c *OpenAIClient) chat(ctx context.Context, apiKey, model, system, user string, maxTokens int) (string, error) {
	if model == "" {
		model = defaultModel
	}
	body, err := json.Marshal(chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		ResponseFormat: chatResponseFormat{Type: "json_object"},
		MaxTokens:      maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("ai: encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: calling OpenAI: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai: reading OpenAI response: %w", err)
	}
	var chat chatResponse
	if err := json.Unmarshal(respBody, &chat); err != nil {
		return "", fmt.Errorf("ai: parsing OpenAI response: %w", err)
	}
	if chat.Error != nil {
		return "", fmt.Errorf("ai: OpenAI rejected the request: %s", chat.Error.Message)
	}
	if len(chat.Choices) == 0 {
		return "", fmt.Errorf("ai: OpenAI returned no choices")
	}
	return chat.Choices[0].Message.Content, nil
}

func capList(l []string) []string {
	if len(l) > maxTasteItems {
		return l[:maxTasteItems]
	}
	return l
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
