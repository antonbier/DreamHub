package autotimer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Shape as produced by AutoTimerConfiguration.buildConfig(webif=True) on
// the reference box's plugin version (read from its source).
const listFixture = `<?xml version="1.0" ?>
<autotimer version="8">
 <defaults id="-1">
 </defaults>
 <timer name="Tatort" match="Tatort" enabled="yes" id="1" justplay="0">
  <e2service>
   <e2servicereference>1:0:19:283D:3FB:1:C00000:0:0:0:</e2servicereference>
   <e2servicename>Das Erste HD</e2servicename>
  </e2service>
 </timer>
 <timer name="Doku" match="Terra X" enabled="no" id="2">
 </timer>
</autotimer>`

const simulateFixture = `<?xml version="1.0" encoding="UTF-8" ?>
<e2autotimersimulate api_version="1.7">
<e2simulatedtimer>
   <e2servicereference>1:0:19:283D:3FB:1:C00000:0:0:0:</e2servicereference>
   <e2servicename>Das Erste HD</e2servicename>
   <e2name>Tatort: Borowski</e2name>
   <e2timebegin>1790449200</e2timebegin>
   <e2timeend>1790455200</e2timeend>
   <e2autotimername>Tatort</e2autotimername>
</e2simulatedtimer>
</e2autotimersimulate>`

func TestListAndSimulate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/autotimer":
			w.Write([]byte(listFixture))
		case "/autotimer/simulate":
			w.Write([]byte(simulateFixture))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)

	list, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "Tatort" || !list[0].Enabled || list[1].Enabled || len(list[0].Services) != 1 || list[0].Services[0].ServiceName != "Das Erste HD" {
		t.Errorf("unexpected list: %+v", list)
	}
	sim, err := c.Simulate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sim) != 1 || sim[0].Name != "Tatort: Borowski" || sim[0].Begin != 1790449200 || sim[0].AutoTimerName != "Tatort" {
		t.Errorf("unexpected simulation: %+v", sim)
	}
}

func TestNotInstalled(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := NewHTTPClient(srv.URL).List(context.Background()); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("want ErrNotInstalled, got %v", err)
	}
}
