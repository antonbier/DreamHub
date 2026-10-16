package connector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	srmock "github.com/antonbier/DreamHub/backend/internal/serienrecorder/mock"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

type counting struct{ calls atomic.Int32 }

func (*counting) ID() string                   { return "counting" }
func (*counting) Name() string                 { return "Counting" }
func (*counting) Settings() []settings.Section { return nil }
func (c *counting) Check(context.Context, settings.Values) Report {
	c.calls.Add(1)
	return Report{State: OK}
}

func TestReports_CachedAndInvalidatedBySettingsVersion(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	c := &counting{}
	r.Add(c)
	ctx := context.Background()

	r.Reports(ctx, settings.Values{}, 1)
	r.Reports(ctx, settings.Values{}, 1)
	if n := c.calls.Load(); n != 1 {
		t.Fatalf("second call within TTL must be cached, got %d checks", n)
	}
	r.Reports(ctx, settings.Values{}, 2)
	if n := c.calls.Load(); n != 2 {
		t.Fatalf("a settings change must trigger a fresh check, got %d checks", n)
	}
	now = now.Add(31 * time.Second)
	r.Reports(ctx, settings.Values{}, 2)
	if n := c.calls.Load(); n != 3 {
		t.Fatalf("expired cache must trigger a fresh check, got %d checks", n)
	}
}

type failingClient struct{ err error }

func (f failingClient) DeviceInfo(context.Context) (*webcontrol.DeviceInfo, error) {
	return nil, f.fail()
}
func (f failingClient) LiveStream(context.Context, string) (io.ReadCloser, error) {
	return nil, f.fail()
}
func (f failingClient) RecordingFile(context.Context, string, string) (*webcontrol.FileResponse, error) {
	return nil, f.fail()
}
func (f failingClient) MovieList(context.Context) ([]webcontrol.Movie, error) {
	return nil, f.fail()
}
func (f failingClient) TimerList(context.Context) ([]webcontrol.Timer, error) {
	return nil, f.fail()
}
func (f failingClient) AddTimer(context.Context, webcontrol.NewTimer) error {
	return f.fail()
}
func (f failingClient) DeleteTimer(context.Context, webcontrol.TimerRef) error {
	return f.fail()
}
func (f failingClient) EPGSearch(context.Context, string) ([]webcontrol.EPGEvent, error) {
	return nil, f.fail()
}
func (f failingClient) Locations(context.Context) ([]string, error) { return nil, f.fail() }
func (f failingClient) MovieDir(context.Context, string) (webcontrol.MovieFolder, error) {
	return webcontrol.MovieFolder{}, f.fail()
}
func (f failingClient) CurrentServiceReference(context.Context) (string, error) { return "", f.fail() }
func (f failingClient) Zap(context.Context, string) error                       { return f.fail() }
func (f failingClient) PlayMovie(context.Context, string, string, int64) error  { return f.fail() }
func (f failingClient) SendKey(context.Context, int) error                      { return f.fail() }
func (f failingClient) Message(context.Context, string, int, int) error         { return f.fail() }
func (f failingClient) Screenshot(context.Context) ([]byte, error)              { return nil, f.fail() }
func (f failingClient) InStandby(context.Context) (bool, error)                 { return false, f.fail() }
func (f failingClient) Current(context.Context) (*webcontrol.CurrentService, error) {
	return nil, f.fail()
}
func (f failingClient) fail() error {
	if f.err != nil {
		return f.err
	}
	return errors.New("connection refused")
}
func (f failingClient) Bouquets(context.Context) ([]webcontrol.Service, error) {
	return nil, f.fail()
}
func (f failingClient) Services(context.Context, string) ([]webcontrol.Service, error) {
	return nil, f.fail()
}
func (f failingClient) EPGNowNext(context.Context, string) ([]webcontrol.NowNext, error) {
	return nil, f.fail()
}
func (f failingClient) EPGMulti(context.Context, string, int64, int64) ([]webcontrol.EPGEvent, error) {
	return nil, f.fail()
}

func TestWebControl(t *testing.T) {
	ctx := context.Background()
	if rep := (WebControl{Client: mock.New()}).Check(ctx, nil); rep.State != OK {
		t.Errorf("mock WebControl must be OK, got %+v", rep)
	}
	rep := WebControl{Client: failingClient{}}.Check(ctx, nil)
	if rep.State != Unreachable || rep.Problem == nil || rep.Problem.Code != status.CodeConnectorUnreachable || rep.Problem.Fix != status.FixCheckBoxConnection {
		t.Errorf("unreachable box must report a problem with a fix, got %+v", rep)
	}

	// WebControl asking for a password (localauth=true on the box) is a
	// different problem with a different fix than an unreachable box.
	rep = WebControl{Client: failingClient{err: fmt.Errorf("x: %w", webcontrol.ErrUnauthorized)}}.Check(ctx, nil)
	if rep.State != NotConfigured || rep.Problem == nil || rep.Problem.Code != status.CodeConnectorAuthRequired || rep.Problem.Fix != status.FixConfigureInSettings || rep.Problem.FixParams["section"] != "webcontrol" {
		t.Errorf("a 401 from WebControl must point at the credentials setting, got %+v", rep)
	}
}

func TestKeyedConnectors(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		conn    Connector
		key     string
		section string
	}{
		{OpenAI{}, KeyOpenAIAPIKey, "ai"},
		{TMDB{}, KeyTMDBAPIKey, "tmdb"},
		{OMDb{}, KeyOMDbAPIKey, "omdb"},
		{Fanart{}, KeyFanartAPIKey, "fanart"},
	} {
		rep := c.conn.Check(ctx, settings.Values{})
		if rep.State != NotConfigured || rep.Problem.FixParams["section"] != c.section {
			t.Errorf("%s without key: expected not_configured pointing to section %q, got %+v", c.conn.Name(), c.section, rep)
		}
		if rep := c.conn.Check(ctx, settings.Values{c.key: "some-key"}); rep.State != OK {
			t.Errorf("%s with key: expected ok, got %+v", c.conn.Name(), rep)
		}
	}
}

type failingSeriesClient struct{}

func (failingSeriesClient) Info(context.Context) (*serienrecorder.Info, error) {
	return nil, errors.New("connection refused")
}
func (failingSeriesClient) Markers(context.Context) ([]serienrecorder.Marker, error) {
	return nil, errors.New("connection refused")
}
func (failingSeriesClient) SeriesInfo(context.Context, string, string) (*serienrecorder.SeriesInfo, error) {
	return nil, errors.New("unreachable")
}
func (failingSeriesClient) UpcomingEpisodes(context.Context) ([]serienrecorder.UpcomingEpisode, error) {
	return nil, errors.New("connection refused")
}

func TestSerienRecorder_RealReachabilityCheck(t *testing.T) {
	ctx := context.Background()
	if rep := (SerienRecorder{Mock: true}).Check(ctx, nil); rep.State != OK {
		t.Errorf("mock mode: expected ok, got %+v", rep)
	}
	if rep := (SerienRecorder{Client: srmock.New()}).Check(ctx, nil); rep.State != OK {
		t.Errorf("live mode with a reachable API: expected ok, got %+v", rep)
	}
	rep := (SerienRecorder{Client: failingSeriesClient{}}).Check(ctx, nil)
	if rep.State != Unreachable || rep.Problem == nil || rep.Problem.Code != status.CodeConnectorUnreachable || rep.Problem.Fix != status.FixCheckBoxConnection {
		t.Errorf("unreachable API must report a problem with a fix, got %+v", rep)
	}
}

// An optional plugin that isn't installed is a normal state with its own
// explanation, never "check the connection" (docs/archive/V1_PLAN.md, rule 1).
func TestSerienRecorder_NotInstalled(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	rep := (SerienRecorder{Client: serienrecorder.NewHTTPClient(srv.URL)}).Check(context.Background(), nil)
	if rep.State != NotInstalled || rep.Problem == nil || rep.Problem.Code != status.CodeConnectorNotInstalled || rep.Problem.Fix != status.FixInstallPlugin {
		t.Errorf("a missing SerienRecorder must report not_installed with an install hint, got %+v", rep)
	}
}

func TestReports_AreCachedPerBox(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	c := &counting{}
	r.Add(c)
	main, other := box.With(context.Background(), "main"), box.With(context.Background(), "bedroom")
	r.Reports(main, settings.Values{}, 1)
	r.Reports(other, settings.Values{}, 1)
	if n := c.calls.Load(); n != 2 {
		t.Fatalf("each box needs its own check, got %d", n)
	}
	r.Reports(main, settings.Values{}, 1)
	r.Reports(other, settings.Values{}, 1)
	if n := c.calls.Load(); n != 2 {
		t.Fatalf("both are cached, got %d checks", n)
	}
}
