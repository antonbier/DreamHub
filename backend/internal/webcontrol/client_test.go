package webcontrol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
)

// timerListXML follows the OpenWebif schema; to be replaced by a real
// fixture once captured from the device (#14).
const timerListXML = `<?xml version="1.0" encoding="UTF-8"?>
<e2timerlist>
	<e2timer>
		<e2servicereference>1:0:19:2EE3:441:1:C00000:0:0:0:</e2servicereference>
		<e2servicename>RTL Television</e2servicename>
		<e2name>Bauer sucht Frau International</e2name>
		<e2description>Folge 8</e2description>
		<e2timebegin>1790100600</e2timebegin>
		<e2timeend>1790104200</e2timeend>
		<e2disabled>0</e2disabled>
		<e2state>0</e2state>
	</e2timer>
</e2timerlist>`

func TestTimerList_Parses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/timerlist" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(timerListXML))
	}))
	defer srv.Close()

	timers, err := NewHTTPClient(srv.URL).TimerList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 1 || timers[0].Name != "Bauer sucht Frau International" || timers[0].Begin != 1790100600 || timers[0].Disabled {
		t.Errorf("unexpected parse result: %+v", timers)
	}
}

func TestAddTimer_DisabledByDefault(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()

	err := NewHTTPClient(srv.URL).AddTimer(context.Background(), NewTimer{ServiceReference: "x", Name: "y", Begin: 1, End: 2})
	if !errors.Is(err, ErrWritesDisabled) || called {
		t.Fatalf("writes must be refused without contacting the box, got %v (called=%v)", err, called)
	}
}

const okResult = `<e2simplexmlresult><e2state>True</e2state><e2statetext>ok</e2statetext></e2simplexmlresult>`

func TestAddTimer_PostsEveryParameter(t *testing.T) {
	var method string
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		r.ParseForm()
		form = r.PostForm
		w.Write([]byte(okResult))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	err := c.AddTimer(context.Background(), NewTimer{ServiceReference: "1:0:19:1:1:1:C00000:0:0:0:", Name: "Die Kanzlei", Begin: 100, End: 200, EventID: 4711})
	if err != nil {
		t.Fatal(err)
	}
	// POST, because the timer screens refuse GET with anti_hijack on; and
	// every parameter present (even empty ones), because a missing one
	// reaches the plugin as None and crashes it (ARCHITECTURE.md).
	if method != http.MethodPost {
		t.Errorf("timeradd must be a POST, got %s", method)
	}
	for _, k := range []string{"sRef", "begin", "end", "name", "description", "repeated", "dirname", "tags", "eit", "disabled", "justplay", "afterevent"} {
		if _, ok := form[k]; !ok {
			t.Errorf("parameter %q missing from timeradd", k)
		}
	}
	if form.Get("sRef") != "1:0:19:1:1:1:C00000:0:0:0:" || form.Get("begin") != "100" || form.Get("end") != "200" || form.Get("name") != "Die Kanzlei" || form.Get("eit") != "4711" {
		t.Errorf("unexpected timeradd parameters: %v", form)
	}
}

func TestDeleteTimer_PostsExactReference(t *testing.T) {
	var path string
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		r.ParseForm()
		form = r.PostForm
		w.Write([]byte(okResult))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	if err := c.DeleteTimer(context.Background(), TimerRef{ServiceReference: "1:0:1:2EE3:441:1:C00000:0:0:0:", Begin: 10, End: 20}); err != nil {
		t.Fatal(err)
	}
	if path != "/web/timerdelete" || form.Get("sRef") != "1:0:1:2EE3:441:1:C00000:0:0:0:" || form.Get("begin") != "10" || form.Get("end") != "20" {
		t.Errorf("unexpected timerdelete request: %s %v", path, form)
	}
}

func TestDeleteTimer_DisabledByDefault(t *testing.T) {
	err := NewHTTPClient("http://127.0.0.1:1").DeleteTimer(context.Background(), TimerRef{})
	if !errors.Is(err, ErrWritesDisabled) {
		t.Fatalf("want ErrWritesDisabled, got %v", err)
	}
}

func TestClient_SendsCredentialsAndReportsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "root" || p != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(timerListXML))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	if _, err := c.TimerList(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("without credentials: want ErrUnauthorized, got %v", err)
	}
	c.Credentials = func() (string, string) { return "root", "secret" }
	if _, err := c.TimerList(context.Background()); err != nil {
		t.Fatalf("with credentials: %v", err)
	}
}

func TestClient_RetriesWithSessionOn412(t *testing.T) {
	// With extended_security on, the box answers 412 until the request
	// carries the sessionid from /web/session (ARCHITECTURE.md).
	sessionCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.URL.Path == "/web/session" {
			sessionCalls++
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><e2sessionid>abc123</e2sessionid>`))
			return
		}
		if r.Form.Get("sessionid") != "abc123" {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		w.Write([]byte(timerListXML))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	for i := 0; i < 2; i++ {
		if _, err := c.TimerList(context.Background()); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if sessionCalls != 1 {
		t.Errorf("the session should be fetched once and reused, fetched %d times", sessionCalls)
	}
}

func TestParseSize(t *testing.T) {
	var gib float64 = 1 << 30
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"2 TB", 2 << 40, true},
		{"263.947 GB", int64(263.947 * gib), true},
		{"1MB", 1 << 20, true},
		{"", 0, false},
		{"unknown", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseSize(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseSize(%q) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestMovieList_ToleratesAMalformedTail(t *testing.T) {
	// Real, confirmed-live response, 2026-09-24 (ARCHITECTURE.md): the
	// reference device's own /web/movielist ended mid-document with an
	// internal "Processing Failed" HTML fragment instead of a properly
	// closed </e2movielist> — not a request-side problem, WebControl's own
	// bug. Losing every recording over one broken entry near the end would
	// be worse than returning the ones that parsed fine.
	fixture, err := os.ReadFile("../../testdata/fixtures/movielist_malformed.xml")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	movies, err := NewHTTPClient(srv.URL).MovieList(context.Background())
	if err != nil {
		t.Fatalf("a malformed tail must not surface as an error: %v", err)
	}
	if len(movies) == 0 {
		t.Fatal("expected at least the movies that parsed before the malformed tail")
	}
	for _, m := range movies {
		if m.Title == "" {
			t.Errorf("a partially-decoded movie shouldn't appear in the result: %+v", m)
		}
	}
}

func TestAddTimer_BoxRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<e2simplexmlresult><e2state>False</e2state><e2statetext>Conflicting timer</e2statetext></e2simplexmlresult>`))
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	err := c.AddTimer(context.Background(), NewTimer{})
	var rej *RejectedError
	if !errors.As(err, &rej) || rej.Text != "Conflicting timer" {
		t.Fatalf("a rejection by the box must surface as a RejectedError with its text, got %v", err)
	}
}

func TestEPGMulti_SendsEndTimeAsMinutes(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`<e2eventlist></e2eventlist>`))
	}))
	defer srv.Close()
	// A 4-hour window must reach the box as endTime=240 (minutes), not as
	// an end timestamp — which the box reads as "the whole EPG".
	if _, err := NewHTTPClient(srv.URL).EPGMulti(context.Background(), "bouquet", 1000, 1000+4*3600); err != nil {
		t.Fatal(err)
	}
	if got.Get("time") != "1000" || got.Get("endTime") != "240" {
		t.Errorf("want time=1000 endTime=240, got %v", got)
	}
}

func TestMovieDir_ToleratesNoneAndListsSubfolders(t *testing.T) {
	// Real shape (2026-09-24): a manually copied .mkv with e2filesize
	// "None" in the middle of the list, subfolders after it.
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<e2movielist>
	<e2movie><e2title>A</e2title><e2time>-1</e2time><e2filename>//media/hdd/movie/a.ts</e2filename><e2filesize>100</e2filesize></e2movie>
	<e2movie><e2title>Oskars Kleid.mkv</e2title><e2time>None</e2time><e2filename>//media/hdd/movie/Oskars Kleid.mkv</e2filename><e2filesize>None</e2filesize></e2movie>
	<e2movie><e2title>B</e2title><e2time>1780944521</e2time><e2filename>//media/hdd/movie/b.ts</e2filename><e2filesize>200</e2filesize></e2movie>
	<e2locations>
		<e2location>/media/hdd/movie/Film/</e2location>
		<e2location>/media/hdd/movie/serien/</e2location>
	</e2locations>
</e2movielist>`
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		form = r.PostForm
		w.Write([]byte(body))
	}))
	defer srv.Close()
	f, err := NewHTTPClient(srv.URL).MovieDir(context.Background(), "/media/hdd/movie/")
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("dirname") != "/media/hdd/movie/" {
		t.Errorf("dirname must always be sent explicitly, got %v", form)
	}
	if len(f.Movies) != 3 || f.Movies[1].FileSize != 0 || f.Movies[2].FileSize != 200 || f.Movies[2].Time != 1780944521 {
		t.Errorf("a \"None\" field must not lose the entry or the ones after it: %+v", f.Movies)
	}
	if len(f.Subfolders) != 2 || f.Subfolders[1] != "/media/hdd/movie/serien/" {
		t.Errorf("subfolders after the odd entry must survive: %v", f.Subfolders)
	}
}

func TestGet_CoalescesConcurrentIdenticalRequests(t *testing.T) {
	var hits int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-release // every concurrent caller must already be waiting before this returns
		w.Write([]byte(`<e2movielist><e2movie><e2title>x</e2title></e2movie></e2movielist>`))
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)

	const callers = 5
	var wg sync.WaitGroup
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = c.MovieList(context.Background())
		}(i)
	}
	// Give every goroutine a chance to reach the server before releasing it —
	// if coalescing didn't work, a second request would also block here,
	// and this call count check would eventually flake, not deadlock.
	for atomic.LoadInt32(&hits) == 0 {
	}
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected exactly 1 request to the box for %d concurrent identical callers, got %d", callers, got)
	}
}
