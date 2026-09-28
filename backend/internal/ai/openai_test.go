package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeOpenAI(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("expected the API key as a Bearer token, got %q", r.Header.Get("Authorization"))
		}
		resp := chatResponse{Choices: []struct {
			Message chatMessage `json:"message"`
		}{{Message: chatMessage{Content: content}}}}
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestRecommend_GroundsPicksInTheGivenItems(t *testing.T) {
	srv := fakeOpenAI(t, `{"picks":[{"index":1,"reason":"Great cast."},{"index":0,"reason":"Timely."}]}`)
	defer srv.Close()

	c := &OpenAIClient{HTTP: srv.Client(), BaseURL: srv.URL}
	items := []EPGItem{
		{Title: "News", ServiceReference: "ref1", ServiceName: "Chan1", Start: 100, Duration: 60},
		{Title: "Drama", ServiceReference: "ref2", ServiceName: "Chan2", Start: 200, Duration: 120},
	}
	recs, err := c.Recommend(context.Background(), "sk-test", "", items, Taste{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].Title != "Drama" || recs[0].ServiceReference != "ref2" || recs[1].Title != "News" {
		t.Fatalf("recommendations must be built from the original items at the picked index, got %+v", recs)
	}
	if recs[0].Reason != "Great cast." {
		t.Errorf("expected the model's reason to be kept, got %q", recs[0].Reason)
	}
}

func TestRecommend_DropsOutOfRangeIndices(t *testing.T) {
	// A hallucinated or malformed index must never surface as a
	// recommendation — it would create a real timer for the wrong program.
	srv := fakeOpenAI(t, `{"picks":[{"index":5,"reason":"nonexistent"},{"index":-1,"reason":"nonexistent"},{"index":0,"reason":"real"}]}`)
	defer srv.Close()

	c := &OpenAIClient{HTTP: srv.Client(), BaseURL: srv.URL}
	items := []EPGItem{{Title: "Only item", ServiceReference: "ref1", ServiceName: "Chan1", Start: 100, Duration: 60}}
	recs, err := c.Recommend(context.Background(), "sk-test", "", items, Taste{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Title != "Only item" || recs[0].Reason != "real" {
		t.Fatalf("expected only the in-range pick to survive, got %+v", recs)
	}
}

func TestRecommend_NoAPIKey(t *testing.T) {
	c := NewOpenAIClient()
	if _, err := c.Recommend(context.Background(), "", "", []EPGItem{{Title: "x"}}, Taste{}); err != ErrNotConfigured {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestRecommend_NoItemsIsNotAnError(t *testing.T) {
	c := NewOpenAIClient()
	recs, err := c.Recommend(context.Background(), "sk-test", "", nil, Taste{})
	if err != nil || recs != nil {
		t.Errorf("expected (nil, nil) for an empty candidate list, got (%v, %v)", recs, err)
	}
}

func TestSuggestName_ValidatesTheAnswer(t *testing.T) {
	answer := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": answer}}}})
		w.Write(b)
	}))
	defer srv.Close()
	c := &OpenAIClient{HTTP: srv.Client(), BaseURL: srv.URL}
	in := NameInput{FileName: "20260923 1932 - RTL - GZSZ.ts", Title: "GZSZ"}

	answer = `{"title":"Gute Zeiten, schlechte Zeiten","year":0,"season":0,"episode":8614,"episodeTitle":"Hirngespinst"}`
	f, err := c.SuggestName(context.Background(), "key", "", in)
	if err != nil || f.Title != "Gute Zeiten, schlechte Zeiten" || f.Episode != 8614 {
		t.Fatalf("valid answer: %+v, %v", f, err)
	}
	for _, bad := range []string{`{"title":""}`, `{"title":"X","year":20}`, `not json`} {
		answer = bad
		if _, err := c.SuggestName(context.Background(), "key", "", in); !errors.Is(err, ErrInvalidAnswer) {
			t.Errorf("%s: want ErrInvalidAnswer, got %v", bad, err)
		}
	}
}

func TestRecommend_SendsTheTasteOnlyWhenThereIsOne(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		bodies = append(bodies, req.Messages[len(req.Messages)-1].Content)
		json.NewEncoder(w).Encode(chatResponse{Choices: []struct {
			Message chatMessage `json:"message"`
		}{{Message: chatMessage{Content: `{"picks":[]}`}}}})
	}))
	defer srv.Close()
	c := &OpenAIClient{HTTP: srv.Client(), BaseURL: srv.URL}
	items := []EPGItem{{Title: "Polizeiruf 110", ServiceReference: "ref1", ServiceName: "Das Erste HD", Start: 100, Duration: 5400}}

	if _, err := c.Recommend(context.Background(), "sk-test", "", items, Taste{Series: []string{"Tatort"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Recommend(context.Background(), "sk-test", "", items, Taste{}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[0], `"taste":{"series":["Tatort"]}`) || !strings.Contains(bodies[0], `"candidates":[`) {
		t.Errorf("expected candidates plus the taste, got %q", bodies)
	}
	if strings.Contains(bodies[1], `"taste"`) {
		t.Errorf("an empty taste must not be sent at all, got %q", bodies[1])
	}
}
