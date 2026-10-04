package webcontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCurrent_Fixture(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/fixtures/getcurrent.xml")
	if err != nil {
		t.Fatal(err)
	}
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write(fixture)
	}))
	defer srv.Close()

	cur, err := NewHTTPClient(srv.URL).Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if path != "/web/getcurrent" {
		t.Errorf("asked %q", path)
	}
	if cur.ServiceReference != "1:0:19:14:280E:217C:EEEE01DC:0:0:0:" || cur.ServiceName != "RAS ORF1 HD" || cur.ProviderName != "RAS" {
		t.Errorf("service: %+v", cur)
	}
	if cur.Now == nil || cur.Now.Title != "WaPo Bodensee" || cur.Now.Start != 1790231091 || cur.Now.Duration != 2928 {
		t.Errorf("now: %+v", cur.Now)
	}
	if cur.Next == nil || cur.Next.Start != 1790234019 || cur.Next.ServiceName != "RAS ORF1 HD" {
		t.Errorf("next: %+v", cur.Next)
	}
	if cur.Volume != 100 || cur.Muted {
		t.Errorf("volume: %d muted %v", cur.Volume, cur.Muted)
	}
}

func TestCurrent_NoEPG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<e2currentserviceinformation><e2service><e2servicereference>1:0:2:7008:453:1:C00000:0:0:0:</e2servicereference><e2servicename>Klassik Radio</e2servicename></e2service>
<e2eventlist><e2event><e2eventid>None</e2eventid><e2eventstart>None</e2eventstart><e2eventduration>None</e2eventduration><e2eventtitle>None</e2eventtitle></e2event></e2eventlist>
<e2volume><e2current>40</e2current><e2ismuted>True</e2ismuted></e2volume></e2currentserviceinformation>`))
	}))
	defer srv.Close()
	cur, err := NewHTTPClient(srv.URL).Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cur.Now != nil || cur.Next != nil || cur.Volume != 40 || !cur.Muted {
		t.Errorf("got %+v", cur)
	}
}

func TestIsRecordingReference(t *testing.T) {
	if !IsRecordingReference("1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/a.ts") {
		t.Error("recording not recognized")
	}
	if IsRecordingReference("1:0:19:2EE3:441:1:C00000:0:0:0:") {
		t.Error("live channel taken for a recording")
	}
}
