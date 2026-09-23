package webcontrol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestAddTimer_WhenEnabled(t *testing.T) {
	var query map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		w.Write([]byte(`<e2simplexmlresult><e2state>True</e2state><e2statetext>Timer added</e2statetext></e2simplexmlresult>`))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	err := c.AddTimer(context.Background(), NewTimer{ServiceReference: "1:0:19:1:1:1:C00000:0:0:0:", Name: "Die Kanzlei", Begin: 100, End: 200})
	if err != nil {
		t.Fatal(err)
	}
	if query["sRef"][0] != "1:0:19:1:1:1:C00000:0:0:0:" || query["begin"][0] != "100" || query["end"][0] != "200" || query["name"][0] != "Die Kanzlei" {
		t.Errorf("unexpected timeradd parameters: %v", query)
	}
}

func TestAddTimer_BoxRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<e2simplexmlresult><e2state>False</e2state><e2statetext>Conflicting timer</e2statetext></e2simplexmlresult>`))
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	if err := c.AddTimer(context.Background(), NewTimer{}); err == nil {
		t.Fatal("a rejection by the box must surface as an error")
	}
}
