package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	recs, err := c.Recommend(context.Background(), "sk-test", "", items)
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
	recs, err := c.Recommend(context.Background(), "sk-test", "", items)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Title != "Only item" || recs[0].Reason != "real" {
		t.Fatalf("expected only the in-range pick to survive, got %+v", recs)
	}
}

func TestRecommend_NoAPIKey(t *testing.T) {
	c := NewOpenAIClient()
	if _, err := c.Recommend(context.Background(), "", "", []EPGItem{{Title: "x"}}); err != ErrNotConfigured {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestRecommend_NoItemsIsNotAnError(t *testing.T) {
	c := NewOpenAIClient()
	recs, err := c.Recommend(context.Background(), "sk-test", "", nil)
	if err != nil || recs != nil {
		t.Errorf("expected (nil, nil) for an empty candidate list, got (%v, %v)", recs, err)
	}
}
