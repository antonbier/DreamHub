package connector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

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

type failingClient struct{}

func (failingClient) DeviceInfo(context.Context) (*webcontrol.DeviceInfo, error) {
	return nil, errors.New("connection refused")
}
func (failingClient) MovieList(context.Context) ([]webcontrol.Movie, error) {
	return nil, errors.New("connection refused")
}
func (failingClient) TimerList(context.Context) ([]webcontrol.Timer, error) {
	return nil, errors.New("connection refused")
}
func (failingClient) AddTimer(context.Context, webcontrol.NewTimer) error {
	return errors.New("connection refused")
}
func (failingClient) Bouquets(context.Context) ([]webcontrol.Service, error) {
	return nil, errors.New("connection refused")
}
func (failingClient) Services(context.Context, string) ([]webcontrol.Service, error) {
	return nil, errors.New("connection refused")
}
func (failingClient) EPGMulti(context.Context, string, int64, int64) ([]webcontrol.EPGEvent, error) {
	return nil, errors.New("connection refused")
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
