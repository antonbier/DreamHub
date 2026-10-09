package webcontrol

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// BouquetEditor is the optional BouquetEditor plugin's write API
// (/bouqueteditor/web/*, parameters in ARCHITECTURE.md) — the one thing
// that changes the box's channel list for ChannelHub
// (docs/CHANNELHUB_PLAN.md). Read from the plugin's own XML definitions
// (BouquetEditor.py, confirmed live 2026-10-09): position is a plain index
// passed straight to the native eDVBDB binding with no visible base/header
// handling, so that still needs a real write to confirm; backup and
// restore reload the whole bouquet list and the on-screen service list.
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
	defer c.wrote()
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

// backupNameInvalid mirrors the plugin's own cleanup of a backup's name
// (BouquetEditor.py's backupFiles: any run of characters outside this set
// becomes a single "_") — needed because restore, unlike backup, doesn't
// sanitize or place the name itself; it takes the resulting path literally.
var backupNameInvalid = regexp.MustCompile(`[^A-Za-z0-9_. ]+`)

// EditorRestore reconstructs the path EditorBackup's filename actually
// landed at. The plugin's backup joins BACKUP_PATH ("/tmp"), sanitizes the
// name and appends ".tar" itself; its restore takes the opposite — a
// literal path it checks with exists() — so passing the bare name back
// (as the plugin's own XML parameter suggests) fails every real restore.
func (c *HTTPClient) EditorRestore(ctx context.Context, filename string) error {
	path := "/tmp/" + backupNameInvalid.ReplaceAllString(filename, "_") + ".tar"
	return c.editorWrite(ctx, "restore", url.Values{"Filename": {path}})
}

// AddBouquet names the new bouquet as asked, but the plugin appends
// " (TV)" or " (Radio)" to it itself (BouquetEditor.py's addBouquet,
// unconditionally, confirmed live 2026-10-09) — callers checking the
// result need BouquetDisplayName(name, radio) for the name actually used.
func (c *HTTPClient) AddBouquet(ctx context.Context, name string, radio bool) error {
	return c.editorWrite(ctx, "addbouquet", url.Values{"name": {name}, "mode": {editorMode(radio)}})
}

// BouquetDisplayName is the name AddBouquet's bouquet actually ends up
// with on the box.
func BouquetDisplayName(name string, radio bool) string {
	if radio {
		return name + " (Radio)"
	}
	return name + " (TV)"
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
