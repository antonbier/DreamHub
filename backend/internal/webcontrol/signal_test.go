package webcontrol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// fakeBox answers each path with a fixture file (from testdata/fixtures) or
// an inline body; anything else is a 404.
func fakeBox(t *testing.T, routes map[string]string) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if b, err := os.ReadFile("../../testdata/fixtures/" + body); err == nil {
			body = string(b)
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL)
}

// VTi 15 answers "99 dB" next to "99 %": the percentage again, dropped.
func TestVTi15_TunerSignal(t *testing.T) {
	s, err := fakeBox(t, map[string]string{"/web/signal": "vti15_signal.xml"}).TunerSignal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || s.SNR != 99 || s.AGC != 91 || s.BER != 0 || s.SNRdB != nil {
		t.Errorf("signal: %+v", s)
	}
}

func TestTunerSignal_RealDB(t *testing.T) {
	s, err := fakeBox(t, map[string]string{"/web/signal": `<e2frontendstatus><e2snrdb>12,50 dB</e2snrdb><e2snr>78 %</e2snr><e2ber>3</e2ber><e2acg>88 %</e2acg></e2frontendstatus>`}).TunerSignal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || s.SNR != 78 || s.AGC != 88 || s.BER != 3 || s.SNRdB == nil || *s.SNRdB != 12.5 {
		t.Errorf("signal: %+v", s)
	}
}

func TestTunerSignal_NoNumbersAndMissingCall(t *testing.T) {
	s, err := fakeBox(t, map[string]string{"/web/signal": `<e2frontendstatus><e2snrdb></e2snrdb><e2snr>N/A</e2snr><e2ber></e2ber><e2acg></e2acg></e2frontendstatus>`}).TunerSignal(context.Background())
	if err != nil || s != nil {
		t.Errorf("no numbers: %+v %v", s, err)
	}
	if _, err := fakeBox(t, nil).TunerSignal(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing call: %v", err)
	}
}

// OpenWebif: the JSON form, since its XML form answers "Processing Failed".
func TestVTi15_SimilarEvents(t *testing.T) {
	c := fakeBox(t, map[string]string{
		"/web/deviceinfo": "vti15_deviceinfo.xml",
		"/api/epgsimilar": "vti15_epgsimilar.json",
		"/web/epgsimilar": "<html><head><title>Processing Failed</title></head></html>",
	})
	got, err := c.SimilarEvents(context.Background(), "1:0:19:2840:3FB:1:C00000:0:0:0:", 14153)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture's first entry is the asked event itself: left out.
	if len(got) != 3 || got[0].Title != "Tagesschau" || got[0].Start == 0 || got[0].Duration != 900 || got[0].ServiceName != "SWR RLP HD" || got[0].ID == 14153 {
		t.Errorf("similar: %+v", got)
	}
}

func TestVTi15_SimilarEvents_ErrorObjectIsNoEvents(t *testing.T) {
	c := fakeBox(t, map[string]string{
		"/web/deviceinfo": "vti15_deviceinfo.xml",
		"/api/epgsimilar": `{"result": false, "message": "The parameter 'eventid' must be a number"}`,
	})
	got, err := c.SimilarEvents(context.Background(), "1:0:19:2840:3FB:1:C00000:0:0:0:", 1)
	if err != nil || len(got) != 0 {
		t.Errorf("similar: %+v %v", got, err)
	}
}

// A Dreambox: the XML form.
func TestSimilarEvents_DreamboxXML(t *testing.T) {
	c := fakeBox(t, map[string]string{
		"/web/deviceinfo": "deviceinfo.xml",
		"/web/epgsimilar": `<e2eventlist><e2event><e2eventid>7</e2eventid><e2eventstart>1792778400</e2eventstart><e2eventduration>900</e2eventduration><e2eventtitle>Tagesschau</e2eventtitle><e2eventservicereference>1:0:19:2840:3FB:1:C00000:0:0:0:</e2eventservicereference><e2eventservicename>SWR RLP HD</e2eventservicename></e2event><e2event><e2eventid>8</e2eventid><e2eventservicereference>-1:0:0</e2eventservicereference></e2event></e2eventlist>`,
	})
	got, err := c.SimilarEvents(context.Background(), "1:0:19:283D:3FB:1:C00000:0:0:0:", 5)
	if err != nil || len(got) != 1 || got[0].ID != 7 {
		t.Errorf("similar: %+v %v", got, err)
	}
}
