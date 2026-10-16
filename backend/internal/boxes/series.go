package boxes

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
)

// Series returns the serienrecorder.Client that forwards to the context's
// box. A box whose entry has no SerienRecorder client reports the plugin
// as absent.
func (r *Registry) Series() serienrecorder.Client { return seriesRouter{r} }

type seriesRouter struct{ reg *Registry }

func (c seriesRouter) client(ctx context.Context) (serienrecorder.Client, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	if e.Series == nil {
		return nil, serienrecorder.ErrNotInstalled
	}
	return e.Series, nil
}

func (c seriesRouter) Info(ctx context.Context) (*serienrecorder.Info, error) {
	s, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	return s.Info(ctx)
}

func (c seriesRouter) Markers(ctx context.Context) ([]serienrecorder.Marker, error) {
	s, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	return s.Markers(ctx)
}

func (c seriesRouter) UpcomingEpisodes(ctx context.Context) ([]serienrecorder.UpcomingEpisode, error) {
	s, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	return s.UpcomingEpisodes(ctx)
}

func (c seriesRouter) SeriesInfo(ctx context.Context, wlid, fsid string) (*serienrecorder.SeriesInfo, error) {
	s, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	return s.SeriesInfo(ctx, wlid, fsid)
}

// AutoTimer returns the autotimer.Client that forwards to the context's box.
func (r *Registry) AutoTimer() autotimer.Client { return autoTimerRouter{r} }

type autoTimerRouter struct{ reg *Registry }

func (c autoTimerRouter) client(ctx context.Context) (autotimer.Client, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	if e.AutoTimer == nil {
		return nil, autotimer.ErrNotInstalled
	}
	return e.AutoTimer, nil
}

func (c autoTimerRouter) List(ctx context.Context) ([]autotimer.AutoTimer, error) {
	a, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	return a.List(ctx)
}

func (c autoTimerRouter) Simulate(ctx context.Context) ([]autotimer.SimulatedTimer, error) {
	a, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	return a.Simulate(ctx)
}
