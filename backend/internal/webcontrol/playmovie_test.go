package webcontrol

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlayMovie(t *testing.T) {
	var got http.Header
	var form map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dreamhub/play" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		r.ParseForm()
		got, form = r.Header, r.PostForm
		if r.PostForm.Get("title") == "standby" {
			io.WriteString(w, `{"ok": false, "error": "the box is in standby"}`)
			return
		}
		io.WriteString(w, `{"ok": true}`)
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	ref := "4097:0:1:0:0:0:0:0:0:0:http%3a//127.0.0.1%3a8080/api/v1/plex/stream/t/file.mkv:Das Boot"
	if err := c.PlayMovie(context.Background(), ref, "Das Boot"); err != nil {
		t.Fatal(err)
	}
	if form["sRef"][0] != ref || form["title"][0] != "Das Boot" || got.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("a form POST with sRef and title: %v %v", form, got)
	}
	var rej *RejectedError
	if err := c.PlayMovie(context.Background(), ref, "standby"); !errors.As(err, &rej) || rej.Text != "the box is in standby" {
		t.Errorf("the plugin's refusal: %v", err)
	}
	old := NewHTTPClient(srv.URL + "/none")
	old.AllowWrites = true
	if err := old.PlayMovie(context.Background(), ref, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an older plugin without the path: %v", err)
	}
}
