package webcontrol

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// BouquetEditor is the optional BouquetEditor plugin's write API
// (/bouqueteditor/web/*, parameters in ARCHITECTURE.md) — the one thing
// that changes the box's channel list for ChannelHub
// (docs/CHANNELHUB_PLAN.md). Read from the plugin's own XML definitions;
// how position counts and what a rename of a bouquet needs haven't been
// tried on the box yet, so ChannelHub checks the resulting list after every
// change and restores its backup when it doesn't match.
type BouquetEditor interface {
	BouquetEditorProber
	// EditorBackup and EditorRestore save and bring back the whole channel
	// list (all bouquets) under a file name the plugin chooses the place for.
	EditorBackup(ctx context.Context, filename string) error
	EditorRestore(ctx context.Context, filename string) error
	AddBouquet(ctx context.Context, name string, radio bool) error
	RemoveBouquet(ctx context.Context, bouquetRef string, radio bool) error
	RenameBouquet(ctx context.Context, bouquetRef, newName string, radio bool) error
	// AddToBouquet puts a channel before beforeRef ("" = at the end).
	AddToBouquet(ctx context.Context, bouquetRef, ref, beforeRef, name string) error
	RemoveFromBouquet(ctx context.Context, bouquetRef, ref string) error
	// MoveInBouquet moves an entry to position (0 = first, headings count).
	MoveInBouquet(ctx context.Context, bouquetRef, ref string, position int, radio bool) error
	RenameInBouquet(ctx context.Context, bouquetRef, ref, beforeRef, newName string, radio bool) error
	// AddMarker adds a heading before beforeRef ("" = at the end).
	AddMarker(ctx context.Context, bouquetRef, name, beforeRef string) error
}

func editorMode(radio bool) string {
	if radio {
		return "1"
	}
	return "0"
}

// editorWrite posts one change to the plugin and reads its
// e2simplexmlresult, the same answer format as WebControl's own writes.
func (c *HTTPClient) editorWrite(ctx context.Context, op string, form url.Values) error {
	if !c.AllowWrites {
		return ErrWritesDisabled
	}
	path := "/bouqueteditor/web/" + op
	body, err := c.do(ctx, http.MethodPost, path, form)
	if err != nil {
		return err
	}
	defer body.Close()
	var res simpleResult
	if err := decodeXML(body, path, &res); err != nil {
		return err
	}
	if !res.State {
		return &RejectedError{Op: op, Text: strings.TrimSpace(res.StateText)}
	}
	return nil
}

func (c *HTTPClient) EditorBackup(ctx context.Context, filename string) error {
	return c.editorWrite(ctx, "backup", url.Values{"Filename": {filename}})
}

func (c *HTTPClient) EditorRestore(ctx context.Context, filename string) error {
	return c.editorWrite(ctx, "restore", url.Values{"Filename": {filename}})
}

func (c *HTTPClient) AddBouquet(ctx context.Context, name string, radio bool) error {
	return c.editorWrite(ctx, "addbouquet", url.Values{"name": {name}, "mode": {editorMode(radio)}})
}

func (c *HTTPClient) RemoveBouquet(ctx context.Context, bouquetRef string, radio bool) error {
	return c.editorWrite(ctx, "removebouquet", url.Values{"sBouquetRef": {bouquetRef}, "mode": {editorMode(radio)}})
}

func (c *HTTPClient) RenameBouquet(ctx context.Context, bouquetRef, newName string, radio bool) error {
	root := `1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "bouquets.tv" ORDER BY bouquet`
	if radio {
		root = RadioRoot
	}
	return c.editorWrite(ctx, "renameservice", url.Values{"sBouquetRef": {root}, "sRef": {bouquetRef}, "sRefBefore": {""},
		"newName": {newName}, "mode": {editorMode(radio)}, "oldsRef": {bouquetRef}})
}

func (c *HTTPClient) AddToBouquet(ctx context.Context, bouquetRef, ref, beforeRef, name string) error {
	return c.editorWrite(ctx, "addservicetobouquet", url.Values{"sBouquetRef": {bouquetRef}, "sRef": {ref},
		"sRefBefore": {beforeRef}, "Name": {name}, "Url": {""}})
}

func (c *HTTPClient) RemoveFromBouquet(ctx context.Context, bouquetRef, ref string) error {
	return c.editorWrite(ctx, "removeservice", url.Values{"sBouquetRef": {bouquetRef}, "sRef": {ref}})
}

func (c *HTTPClient) MoveInBouquet(ctx context.Context, bouquetRef, ref string, position int, radio bool) error {
	return c.editorWrite(ctx, "moveservice", url.Values{"sBouquetRef": {bouquetRef}, "sRef": {ref},
		"mode": {editorMode(radio)}, "position": {strconv.Itoa(position)}})
}

func (c *HTTPClient) RenameInBouquet(ctx context.Context, bouquetRef, ref, beforeRef, newName string, radio bool) error {
	return c.editorWrite(ctx, "renameservice", url.Values{"sBouquetRef": {bouquetRef}, "sRef": {ref}, "sRefBefore": {beforeRef},
		"newName": {newName}, "mode": {editorMode(radio)}, "oldsRef": {ref}})
}

func (c *HTTPClient) AddMarker(ctx context.Context, bouquetRef, name, beforeRef string) error {
	return c.editorWrite(ctx, "addmarkertobouquet", url.Values{"sBouquetRef": {bouquetRef}, "Name": {name}, "sRefBefore": {beforeRef}})
}

var _ BouquetEditor = (*HTTPClient)(nil)
