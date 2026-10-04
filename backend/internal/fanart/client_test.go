package fanart

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movies/603":
			w.Write([]byte(`{"hdmovielogo":[
				{"url":"https://assets.fanart.tv/fanart/movies/603/hdmovielogo/en-a.png","lang":"en","likes":"9"},
				{"url":"https://assets.fanart.tv/fanart/movies/603/hdmovielogo/de-b.png","lang":"de","likes":"2"},
				{"url":"https://assets.fanart.tv/fanart/movies/603/hdmovielogo/de-c.png","lang":"de","likes":"5"},
				{"url":"https://evil.example/fanart/movies/603/hdmovielogo/de-d.png","lang":"de","likes":"99"}]}`))
		case "/tv/81189":
			w.Write([]byte(`{"clearlogo":[{"url":"http://assets.fanart.tv/fanart/tv/81189/clearlogo/x.png","lang":"en","likes":"1"}]}`))
		case "/movies/1":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &HTTPClient{HTTP: srv.Client(), BaseURL: srv.URL}
	ctx := context.Background()

	if p, err := c.Logo(ctx, "k", "movie", 603, []string{"de", "en"}); err != nil || p != "/fanart/movies/603/hdmovielogo/de-c.png" {
		t.Errorf("the most liked in the first language, never another host: %q %v", p, err)
	}
	if p, err := c.Logo(ctx, "k", "tv", 81189, []string{"de"}); err != nil || p != "/fanart/tv/81189/clearlogo/x.png" {
		t.Errorf("falls back to the plain logo and another language: %q %v", p, err)
	}
	if p, err := c.Logo(ctx, "k", "movie", 5, nil); err != nil || p != "" {
		t.Errorf("unknown is no logo, no error: %q %v", p, err)
	}
	if _, err := c.Logo(ctx, "k", "movie", 1, nil); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("a rejected key: %v", err)
	}
	if _, err := c.Logo(ctx, "", "movie", 603, nil); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no key: %v", err)
	}
}

func TestValidImage(t *testing.T) {
	for p, ok := range map[string]bool{
		"/fanart/movies/603/hdmovielogo/the-matrix-5245.png": true,
		"/fanart/tv/81189/clearlogo/x.jpg":                   true,
		"/fanart/movies/603/hdmovielogo/../../etc.png":       false,
		"/etc/passwd":                                        false,
		"/fanart/movies/603/hdmovielogo/x.svg":               false,
	} {
		if ValidImage(p) != ok {
			t.Errorf("ValidImage(%q) != %v", p, ok)
		}
	}
}
