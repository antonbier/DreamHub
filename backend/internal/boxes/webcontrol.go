package boxes

import (
	"context"
	"io"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// WebControl returns the webcontrol.Client every hub, the poller and the
// connector share: each call goes to the box the context names (the first
// box when it names none). It also implements webcontrol.BouquetEditor,
// answering "plugin not available" for a box whose client has none.
func (r *Registry) WebControl() *WebControl { return &WebControl{reg: r} }

// WebControl forwards webcontrol.Client calls to the context's box.
type WebControl struct{ reg *Registry }

var (
	_ webcontrol.Client        = (*WebControl)(nil)
	_ webcontrol.BouquetEditor = (*WebControl)(nil)
)

func (c *WebControl) DeviceInfo(ctx context.Context) (*webcontrol.DeviceInfo, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.DeviceInfo(ctx)
}

func (c *WebControl) MovieList(ctx context.Context) ([]webcontrol.Movie, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.MovieList(ctx)
}

func (c *WebControl) TimerList(ctx context.Context) ([]webcontrol.Timer, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.TimerList(ctx)
}

func (c *WebControl) AddTimer(ctx context.Context, t webcontrol.NewTimer) error {
	e, err := c.reg.For(ctx)
	if err != nil {
		return err
	}
	return e.WC.AddTimer(ctx, t)
}

func (c *WebControl) DeleteTimer(ctx context.Context, ref webcontrol.TimerRef) error {
	e, err := c.reg.For(ctx)
	if err != nil {
		return err
	}
	return e.WC.DeleteTimer(ctx, ref)
}

func (c *WebControl) Bouquets(ctx context.Context) ([]webcontrol.Service, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.Bouquets(ctx)
}

func (c *WebControl) Services(ctx context.Context, bouquetRef string) ([]webcontrol.Service, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.Services(ctx, bouquetRef)
}

func (c *WebControl) EPGMulti(ctx context.Context, bouquetRef string, start, end int64) ([]webcontrol.EPGEvent, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.EPGMulti(ctx, bouquetRef, start, end)
}

func (c *WebControl) EPGNowNext(ctx context.Context, bouquetRef string) ([]webcontrol.NowNext, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.EPGNowNext(ctx, bouquetRef)
}

func (c *WebControl) EPGSearch(ctx context.Context, query string) ([]webcontrol.EPGEvent, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.EPGSearch(ctx, query)
}

func (c *WebControl) Locations(ctx context.Context) ([]string, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.Locations(ctx)
}

func (c *WebControl) MovieDir(ctx context.Context, dir string) (webcontrol.MovieFolder, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return webcontrol.MovieFolder{}, err
	}
	return e.WC.MovieDir(ctx, dir)
}

func (c *WebControl) CurrentServiceReference(ctx context.Context) (string, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return "", err
	}
	return e.WC.CurrentServiceReference(ctx)
}

func (c *WebControl) Zap(ctx context.Context, serviceReference string) error {
	e, err := c.reg.For(ctx)
	if err != nil {
		return err
	}
	return e.WC.Zap(ctx, serviceReference)
}

func (c *WebControl) PlayMovie(ctx context.Context, serviceReference, title string, startMs int64) error {
	e, err := c.reg.For(ctx)
	if err != nil {
		return err
	}
	return e.WC.PlayMovie(ctx, serviceReference, title, startMs)
}

func (c *WebControl) SendKey(ctx context.Context, code int) error {
	e, err := c.reg.For(ctx)
	if err != nil {
		return err
	}
	return e.WC.SendKey(ctx, code)
}

func (c *WebControl) Message(ctx context.Context, text string, kind, timeout int) error {
	e, err := c.reg.For(ctx)
	if err != nil {
		return err
	}
	return e.WC.Message(ctx, text, kind, timeout)
}

func (c *WebControl) Screenshot(ctx context.Context) ([]byte, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.Screenshot(ctx)
}

func (c *WebControl) InStandby(ctx context.Context) (bool, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return false, err
	}
	return e.WC.InStandby(ctx)
}

func (c *WebControl) Current(ctx context.Context) (*webcontrol.CurrentService, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.Current(ctx)
}

func (c *WebControl) LiveStream(ctx context.Context, serviceReference string) (io.ReadCloser, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.LiveStream(ctx, serviceReference)
}

func (c *WebControl) RecordingFile(ctx context.Context, path, rangeHeader string) (*webcontrol.FileResponse, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	return e.WC.RecordingFile(ctx, path, rangeHeader)
}

func (c *WebControl) EditorBackup(ctx context.Context, filename string) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.EditorBackup(ctx, filename)
}

func (c *WebControl) EditorRestore(ctx context.Context, filename string) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.EditorRestore(ctx, filename)
}

func (c *WebControl) AddBouquet(ctx context.Context, name string, radio bool) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.AddBouquet(ctx, name, radio)
}

func (c *WebControl) RemoveBouquet(ctx context.Context, bouquetRef string, radio bool) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.RemoveBouquet(ctx, bouquetRef, radio)
}

func (c *WebControl) RenameBouquet(ctx context.Context, bouquetRef, newName string, radio bool) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.RenameBouquet(ctx, bouquetRef, newName, radio)
}

func (c *WebControl) AddToBouquet(ctx context.Context, bouquetRef, ref, beforeRef, name string) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.AddToBouquet(ctx, bouquetRef, ref, beforeRef, name)
}

func (c *WebControl) RemoveFromBouquet(ctx context.Context, bouquetRef, ref string) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.RemoveFromBouquet(ctx, bouquetRef, ref)
}

func (c *WebControl) MoveInBouquet(ctx context.Context, bouquetRef, ref string, position int, radio bool) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.MoveInBouquet(ctx, bouquetRef, ref, position, radio)
}

func (c *WebControl) RenameInBouquet(ctx context.Context, bouquetRef, ref, beforeRef, newName string, radio bool) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.RenameInBouquet(ctx, bouquetRef, ref, beforeRef, newName, radio)
}

func (c *WebControl) AddMarker(ctx context.Context, bouquetRef, name, beforeRef string) error {
	ed, err := c.editor(ctx)
	if err != nil {
		return err
	}
	return ed.AddMarker(ctx, bouquetRef, name, beforeRef)
}

// BouquetEditorAvailable implements webcontrol.BouquetEditorProber.
func (c *WebControl) BouquetEditorAvailable(ctx context.Context) bool {
	ed, err := c.editor(ctx)
	if err != nil {
		return false
	}
	return ed.BouquetEditorAvailable(ctx)
}

// errNoEditor: the box's client has no BouquetEditor support at all.
var errNoEditor = webcontrol.ErrNotFound

func (c *WebControl) editor(ctx context.Context) (webcontrol.BouquetEditor, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	ed, ok := e.WC.(webcontrol.BouquetEditor)
	if !ok {
		return nil, errNoEditor
	}
	return ed, nil
}

// Raw passes a raw GET through for the self-test's captured answers; a
// client that can't give them reports that as not found, which the
// self-test ignores.
func (c *WebControl) Raw(ctx context.Context, path string) ([]byte, error) {
	e, err := c.reg.For(ctx)
	if err != nil {
		return nil, err
	}
	raw, ok := e.WC.(interface {
		Raw(ctx context.Context, path string) ([]byte, error)
	})
	if !ok {
		return nil, webcontrol.ErrNotFound
	}
	return raw.Raw(ctx, path)
}
