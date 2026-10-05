package plex

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Answers shaped like a Plex Media Server's (1.4x) and plex.tv's v2 API,
// trimmed to the fields the client reads.
const sectionsJSON = `{"MediaContainer":{"size":3,"Directory":[
 {"key":"1","type":"movie","title":"Filme","agent":"tv.plex.agents.movie"},
 {"key":"2","type":"show","title":"Serien"},
 {"key":"3","type":"artist","title":"Musik"}]}}`

const moviesJSON = `{"MediaContainer":{"size":1,"Metadata":[{"ratingKey":"101","key":"/library/metadata/101","type":"movie",
 "title":"Das Boot","year":1981,"summary":"U 96.","thumb":"/library/metadata/101/thumb/1699999999","art":"/library/metadata/101/art/1699999999",
 "audienceRating":8.4,"rating":7.9,"Rating":[{"image":"imdb://image.rating","type":"audience","value":8.4},{"image":"rottentomatoes://image.rating.ripe","type":"critic","value":9.8}],
 "Image":[{"alt":"Das Boot","type":"coverPoster","url":"/library/metadata/101/thumb/1699999999"},{"alt":"Das Boot","type":"clearLogo","url":"/library/metadata/101/clearLogo/1699999999"}],"Guid":[{"id":"imdb://tt0082096"}],
 "UltraBlurColors":{"topLeft":"1a2b3c"},"contentRating":"de/12","duration":8940000,"viewCount":2,"lastViewedAt":1790000000,"addedAt":1600000000,
 "originallyAvailableAt":"1981-09-17","Genre":[{"tag":"Drama"},{"tag":"War"}],
 "Media":[{"id":1,"duration":8940000,"bitrate":9500,"audioChannels":6,"audioCodec":"ac3","videoCodec":"h264","videoResolution":"1080","container":"mkv",
  "Part":[{"id":555,"key":"/library/parts/555/1699999999/file.mkv","duration":8940000,"file":"/data/movies/Das Boot (1981).mkv","size":9000000000,"container":"mkv"}]}]}]}}`

const episodesJSON = `{"MediaContainer":{"size":1,"Metadata":[{"ratingKey":"2111","parentRatingKey":"211","grandparentRatingKey":"201",
 "type":"episode","title":"Geheimnisse","grandparentTitle":"Dark","parentTitle":"Staffel 1","index":1,"parentIndex":1,
 "grandparentThumb":"/library/metadata/201/thumb/1","viewOffset":600000,"duration":3100000}]}}`

func fakeServer(t *testing.T) (*httptest.Server, *[]string) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.URL.Path != "/identity" && r.Header.Get("X-Plex-Token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/identity":
			io.WriteString(w, `{"MediaContainer":{"size":0,"claimed":true,"machineIdentifier":"abc","version":"1.43.4.10903-e5521bd8c"}}`)
		case r.URL.Path == "/":
			io.WriteString(w, `{"MediaContainer":{"friendlyName":"Unraid","version":"1.43.4"}}`)
		case r.URL.Path == "/library/sections":
			io.WriteString(w, sectionsJSON)
		case r.URL.Path == "/library/sections/1/all":
			io.WriteString(w, moviesJSON)
		case r.URL.Path == "/library/metadata/211/children":
			io.WriteString(w, episodesJSON)
		case r.URL.Path == "/:/scrobble" || r.URL.Path == "/:/unscrobble":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/library/parts/555/1699999999/file.mkv":
			if r.Header.Get("Range") != "bytes=100-" {
				t.Errorf("Range must be passed on, got %q", r.Header.Get("Range"))
			}
			w.Header().Set("Content-Range", "bytes 100-109/110")
			w.WriteHeader(http.StatusPartialContent)
			io.WriteString(w, "0123456789")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestHTTPClient_LibraryAndItems(t *testing.T) {
	srv, _ := fakeServer(t)
	c := NewHTTPClient("test")
	conn := Conn{URL: srv.URL, Token: "tok"}
	ctx := context.Background()

	if id, err := c.Identity(ctx, Conn{URL: srv.URL}); err != nil || id.MachineIdentifier != "abc" {
		t.Fatalf("identity without a token: %+v %v", id, err)
	}
	libs, err := c.Libraries(ctx, conn)
	if err != nil || len(libs) != 3 || libs[1] != (Library{ID: "2", Title: "Serien", Type: "show"}) {
		t.Fatalf("libraries: %+v %v", libs, err)
	}
	films, err := c.Items(ctx, conn, "1")
	if err != nil || len(films) != 1 {
		t.Fatalf("films: %+v %v", films, err)
	}
	f := films[0]
	if f.Title != "Das Boot" || f.Year != 1981 || f.Rating != 8.4 || !f.Watched() || len(f.Genres) != 2 || f.Aired != "1981-09-17" || f.Logo != "/library/metadata/101/clearLogo/1699999999" ||
		len(f.Media) != 1 || f.Media[0].VideoCodec != "h264" || f.Media[0].Parts[0].Key != "/library/parts/555/1699999999/file.mkv" {
		t.Errorf("film: %+v", f)
	}
	eps, err := c.Children(ctx, conn, "211")
	if err != nil || len(eps) != 1 {
		t.Fatalf("episodes: %+v %v", eps, err)
	}
	ep := eps[0]
	if ep.ShowTitle != "Dark" || ep.ShowKey != "201" || ep.SeasonKey != "211" || ep.ParentIndex != 1 || ep.Thumb != "/library/metadata/201/thumb/1" || ep.Watched() || ep.ViewOffset != 600000 {
		t.Errorf("episode: %+v", ep)
	}
	if _, err := c.Libraries(ctx, Conn{URL: srv.URL, Token: "wrong"}); err != ErrUnauthorized {
		t.Errorf("a wrong token: %v", err)
	}
	if _, err := c.Libraries(ctx, Conn{URL: "http://127.0.0.1:1", Token: "tok"}); err == nil || !strings.Contains(err.Error(), ErrUnreachable.Error()) {
		t.Errorf("no server: %v", err)
	}
}

func TestHTTPClient_WatchedAndStream(t *testing.T) {
	srv, calls := fakeServer(t)
	c := NewHTTPClient("test")
	conn := Conn{URL: srv.URL + "/", Token: "tok"}
	ctx := context.Background()
	if err := c.SetWatched(ctx, conn, "212", true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetWatched(ctx, conn, "101", false); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(*calls, "\n")
	if !strings.Contains(got, "GET /:/scrobble?identifier=com.plexapp.plugins.library&key=212") ||
		!strings.Contains(got, "GET /:/unscrobble?identifier=com.plexapp.plugins.library&key=101") {
		t.Errorf("scrobble calls: %s", got)
	}
	resp, err := c.Open(ctx, conn, "/library/parts/555/1699999999/file.mkv", "bytes=100-")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(b) != "0123456789" || resp.Header.Get("Content-Range") == "" {
		t.Errorf("stream: %d %q", resp.StatusCode, b)
	}
}

func TestHTTPClient_PinAndResources(t *testing.T) {
	var checks int
	tv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Client-Identifier") != "dreamhub-x" || r.Header.Get("X-Plex-Product") != Product {
			t.Errorf("plex.tv needs the client identifier and product: %v", r.Header)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/pins":
			if r.URL.Query().Get("strong") != "false" {
				t.Error("a short code for plex.tv/link needs strong=false")
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":42,"code":"AB12","expiresAt":"2026-10-05T20:15:00Z","authToken":null}`)
		case r.URL.Path == "/api/v2/pins/42":
			checks++
			if checks == 1 {
				io.WriteString(w, `{"id":42,"code":"AB12","authToken":null}`)
			} else {
				io.WriteString(w, `{"id":42,"code":"AB12","authToken":"account-token"}`)
			}
		case r.URL.Path == "/api/v2/pins/43":
			http.NotFound(w, r)
		case r.URL.Path == "/api/v2/resources":
			if r.Header.Get("X-Plex-Token") != "account-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			io.WriteString(w, `[{"name":"Unraid","product":"Plex Media Server","provides":"server","clientIdentifier":"abc","owned":true,"accessToken":"server-token",
			  "connections":[{"protocol":"https","address":"192.168.1.10","port":32400,"uri":"https://192-168-1-10.abc.plex.direct:32400","local":true,"relay":false},
			                 {"protocol":"https","uri":"https://plx.example.org:443","local":false,"relay":false}]},
			 {"name":"iPhone","product":"Plex for iOS","provides":"client,player","clientIdentifier":"phone"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer tv.Close()
	c := NewHTTPClient("test")
	c.PlexTV = tv.URL
	ctx := context.Background()
	pin, err := c.CreatePin(ctx, "dreamhub-x")
	if err != nil || pin.ID != 42 || pin.Code != "AB12" || pin.ExpiresAt.IsZero() {
		t.Fatalf("pin: %+v %v", pin, err)
	}
	if tok, err := c.CheckPin(ctx, "dreamhub-x", 42); tok != "" || err != nil {
		t.Errorf("before the code is entered: %q %v", tok, err)
	}
	tok, err := c.CheckPin(ctx, "dreamhub-x", 42)
	if tok != "account-token" || err != nil {
		t.Fatalf("after: %q %v", tok, err)
	}
	if _, err := c.CheckPin(ctx, "dreamhub-x", 43); err != ErrPinExpired {
		t.Errorf("an expired code: %v", err)
	}
	res, err := c.Resources(ctx, "dreamhub-x", tok)
	if err != nil || len(res) != 1 || res[0].AccessToken != "server-token" || len(res[0].Connections) != 2 || !res[0].Connections[0].Local {
		t.Errorf("servers only: %+v %v", res, err)
	}
}
