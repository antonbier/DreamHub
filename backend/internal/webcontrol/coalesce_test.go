package webcontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A caller that gives up must not take the shared answer away from the
// others waiting for the same path (the poller behind a closed tab).
func TestCoalescedGetSurvivesACallerLeaving(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("<e2timerlist></e2timerlist>"))
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)

	leaving, cancel := context.WithCancel(context.Background())
	errLeaving := make(chan error, 1)
	go func() {
		_, err := c.coalescedGet(leaving, "/web/timerlist")
		errLeaving <- err
	}()
	time.Sleep(20 * time.Millisecond)
	staying := make(chan error, 1)
	go func() {
		_, err := c.coalescedGet(context.Background(), "/web/timerlist")
		staying <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-errLeaving; err == nil {
		t.Error("the caller that left should get its own cancellation")
	}
	if err := <-staying; err != nil {
		t.Fatalf("the caller that stayed got %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("%d requests to the box, want 1", hits.Load())
	}
}

// A read after a write never gets an answer that was already on its way
// before the write (verifying a new timer against the old list).
func TestCoalescedGetDoesNotJoinAReadFromBeforeAWrite(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(80 * time.Millisecond)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	go c.coalescedGet(context.Background(), "/web/timerlist") // the poller, before the write
	time.Sleep(20 * time.Millisecond)
	c.wrote()
	if _, err := c.coalescedGet(context.Background(), "/web/timerlist"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("%d requests, want 2: the read after the write joined the older one", hits.Load())
	}
}

// Two presses of the same key reach the box twice.
func TestCommandsAreNeverCoalesced(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte("<e2remotecontrol><e2result>True</e2result><e2resulttext>ok</e2resulttext></e2remotecontrol>"))
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { done <- c.SendKey(context.Background(), 115) }()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("%d key presses reached the box, want 2", hits.Load())
	}
}
