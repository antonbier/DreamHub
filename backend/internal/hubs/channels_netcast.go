package hubs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/netcast"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Netcast's source files, read only (docs/NETCAST_PLAN.md, N3). DreamHub
// lists what the Gemini Netcast plugin has configured on the box; it writes
// nothing — that needs the plugin's write route and comes with box settings
// S3. Logins in a file never leave the backend.

type netcastView struct {
	// Available is false when the box serves no files or has no Netcast file.
	Available bool `json:"available"`
	// Writable is true when the box has a write route for these files and
	// its switch (DreamHub's settings on the TV) is on.
	Writable bool           `json:"writable"`
	Files    []netcast.File `json:"files"`
	// Listed is true when the box listed its directory; false means the
	// well-known names were tried instead, so a file of the person's own
	// may be missing.
	Listed bool `json:"listed"`
	// Unreadable names files that exist but could not be read as Netcast
	// source files.
	Unreadable []string `json:"unreadable"`
}

func (h *Channels) netcastSources(w http.ResponseWriter, r *http.Request) {
	rd, ok := h.WC.(webcontrol.BoxFileReader)
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, netcastView{Files: []netcast.File{}, Unreadable: []string{}})
		return
	}
	view, err := readNetcast(r.Context(), rd)
	if wr, ok := h.WC.(webcontrol.BoxFileWriter); ok {
		view.Writable = wr.CanWriteBoxFiles(r.Context())
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

func readNetcast(ctx context.Context, rd webcontrol.BoxFileReader) (netcastView, error) {
	view := netcastView{Files: []netcast.File{}, Unreadable: []string{}}
	var names []string
	listed := true
	for _, pat := range []string{netcast.ServerPattern, netcast.EPGPattern} {
		got, err := rd.ListBoxFiles(ctx, netcast.Dir, pat)
		if err != nil {
			if ctx.Err() != nil {
				return view, err
			}
			// a box that cannot list (a Dreambox, perhaps) is not an error
			listed = false
			break
		}
		names = append(names, got...)
	}
	if !listed {
		names = append([]string(nil), netcast.KnownFiles...)
	}
	view.Listed = listed
	for _, name := range names {
		raw, err := rd.ReadBoxFile(ctx, path.Join(netcast.Dir, name))
		if errors.Is(err, webcontrol.ErrNotFound) && !listed {
			continue // a well-known name the box does not have
		}
		if err != nil {
			if errors.Is(err, webcontrol.ErrTooLarge) || errors.Is(err, webcontrol.ErrNotFound) {
				view.Unreadable = append(view.Unreadable, name)
				continue
			}
			return view, err
		}
		f, perr := netcast.Parse(name, raw)
		if perr != nil {
			view.Unreadable = append(view.Unreadable, name)
			continue
		}
		view.Files = append(view.Files, *f)
	}
	view.Available = len(view.Files) > 0 || len(view.Unreadable) > 0
	return view, nil
}

// --- writing a source (a plan) -----------------------------------------------------

// Error codes of the Netcast routes.
const (
	CodeNetcastNeedsPlugin = "netcast_needs_plugin" // the box has no write route for its files
	CodeNetcastNoSource    = "netcast_no_source"
	CodeNetcastExists      = "netcast_source_exists"
	CodeNetcastBadFile     = "netcast_bad_file"
	CodeNetcastMismatch    = "netcast_write_mismatch"
	CodeNetcastNoList      = "netcast_list_missing"       // the plugin has not downloaded that list yet
	CodeNetcastUnsupported = "netcast_source_unsupported" // a source with no channel list to pick from (Vavoo, films)
)

// backupSuffix is added to a file's name for the copy kept before an edit; it
// does not end in .json, so the plugin does not read it.
const backupSuffix = ".dreamhub-backup"

type netcastPlanRequest struct {
	Action    string `json:"action"` // "add", "update", "remove"
	Kind      string `json:"kind"`   // "server" or "epg"
	File      string `json:"file"`   // the file to change; for "add" optional (else a new file)
	ID        string `json:"id"`     // the source, for update and remove
	Name      string `json:"name"`
	Address   string `json:"address"`
	MediaType string `json:"mediaType"`
	Desc      string `json:"desc"`
}

// NetcastChange is a line of the preview. Address has no login.
type NetcastChange struct {
	Name    string `json:"name"`
	File    string `json:"file"`
	Kind    string `json:"kind"`
	Address string `json:"address,omitempty"`
}

func contentHash(b []byte, exists bool) string {
	if !exists {
		return "absent"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (h *Channels) planNetcast(w http.ResponseWriter, r *http.Request) {
	var req netcastPlanRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"action", "kind", "file", "id", "name", "address"}`)
		return
	}
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	rd, rok := h.WC.(webcontrol.BoxFileReader)
	wr, wok := h.WC.(webcontrol.BoxFileWriter)
	if !rok || !wok || !wr.CanWriteBoxFiles(ctx) {
		httpx.WriteError(w, r, http.StatusConflict, CodeNetcastNeedsPlugin, "the box has no way to change Netcast's files yet")
		return
	}
	in := netcast.Input{ID: req.ID, Name: req.Name, Address: req.Address, MediaType: req.MediaType, Desc: req.Desc}
	invalid := func(field string) {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was planned", map[string]httpx.FieldError{field: {Code: "invalid"}})
	}
	if req.Kind != "server" && req.Kind != "epg" {
		invalid("kind")
		return
	}
	file := strings.TrimSpace(req.File)
	if file != "" && (strings.Contains(file, "/") || netcast.KindOf(file) != req.Kind) {
		invalid("file")
		return
	}

	// the file as it is now
	var old []byte
	exists := false
	if file != "" {
		b, err := rd.ReadBoxFile(ctx, path.Join(netcast.Dir, file))
		switch {
		case err == nil:
			old, exists = b, true
		case errors.Is(err, webcontrol.ErrNotFound) && req.Action == "add":
		case errors.Is(err, webcontrol.ErrNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, CodeNetcastNoSource, "no such file")
			return
		default:
			httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
			return
		}
	}

	var content []byte
	var err error
	var change plan.Change
	var name string
	switch req.Action {
	case "add":
		var id string
		content, id, err = netcast.Add(req.Kind, old, in)
		if err == nil && file == "" {
			file = netcast.FileName(req.Kind, id)
			if _, rerr := rd.ReadBoxFile(ctx, path.Join(netcast.Dir, file)); rerr == nil {
				err = netcast.ErrExists // the new file's name is taken: a source of that id is there
			}
		}
		name = strings.TrimSpace(req.Name)
		change = plan.Change{Type: "netcast.source.add", After: NetcastChange{Name: name, File: file, Kind: req.Kind, Address: iptv.Display(req.Address)}}
	case "update", "remove":
		if file == "" || req.ID == "" {
			invalid("id")
			return
		}
		before, _ := netcast.Find(file, old, req.ID)
		name = before.Name
		if req.Action == "update" {
			content, err = netcast.Update(req.Kind, old, req.ID, in)
			after := NetcastChange{Name: firstOf(strings.TrimSpace(req.Name), before.Name), File: file, Kind: req.Kind, Address: firstOf(iptv.Display(req.Address), before.Address)}
			change = plan.Change{Type: "netcast.source.update", Before: NetcastChange{Name: before.Name, File: file, Kind: req.Kind, Address: before.Address}, After: after}
		} else {
			content, err = netcast.Remove(req.Kind, old, req.ID)
			change = plan.Change{Type: "netcast.source.remove", Before: NetcastChange{Name: before.Name, File: file, Kind: req.Kind, Address: before.Address}}
		}
	default:
		invalid("action")
		return
	}
	switch {
	case errors.Is(err, netcast.ErrInvalid):
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was planned", map[string]httpx.FieldError{"source": {Code: "invalid"}})
		return
	case errors.Is(err, netcast.ErrExists):
		httpx.WriteError(w, r, http.StatusConflict, CodeNetcastExists, "a source with that id exists already")
		return
	case errors.Is(err, netcast.ErrNoSource):
		httpx.WriteError(w, r, http.StatusNotFound, CodeNetcastNoSource, "no such source")
		return
	case errors.Is(err, netcast.ErrBadFile):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, CodeNetcastBadFile, "that file is not a source list DreamHub can edit")
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not prepare the change")
		return
	}

	full := path.Join(netcast.Dir, file)
	summary := "Netcast sources: " + req.Action + " " + name
	pl := h.Plans.Create(ctx, user, plan.Draft{
		Action:      "channels.netcast." + req.Action,
		Changes:     []plan.Change{change},
		Summary:     summary,
		Detail:      map[string]any{"file": file, "kind": req.Kind, "source": name},
		Fingerprint: contentHash(old, exists),
		Current: func(ctx context.Context) (string, error) {
			b, err := rd.ReadBoxFile(ctx, full)
			if errors.Is(err, webcontrol.ErrNotFound) {
				return contentHash(nil, false), nil
			}
			if err != nil {
				return "", err
			}
			return contentHash(b, true), nil
		},
		Execute: func(ctx context.Context) error {
			// a copy of what is there, then the new content, then a look
			// at what the box now holds
			if exists {
				if err := wr.WriteBoxFile(ctx, full+backupSuffix, old); err != nil {
					return netcastWriteFailure(err)
				}
			}
			if err := wr.WriteBoxFile(ctx, full, content); err != nil {
				return netcastWriteFailure(err)
			}
			got, err := rd.ReadBoxFile(ctx, full)
			if err != nil || !bytes.Equal(got, content) {
				if exists {
					_ = wr.WriteBoxFile(ctx, full, old) // put it back
				}
				return &plan.Failure{Status: http.StatusBadGateway, Code: CodeNetcastMismatch, Message: "the box does not hold what was written", Err: err}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "file": file})
}

func netcastWriteFailure(err error) error {
	if errors.Is(err, webcontrol.ErrNeedsPlugin) || errors.Is(err, webcontrol.ErrWritesOff) {
		return &plan.Failure{Status: http.StatusConflict, Code: CodeNetcastNeedsPlugin, Message: "the box has no way to change Netcast's files yet", Err: err}
	}
	return err
}

func firstOf(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

// --- a Netcast playlist as a list for the IPTV tab ----------------------------------------

// netcastCacheDir is where the plugin keeps the lists it has downloaded
// (ARCHITECTURE.md): playlist_netcastserver_<id> is the M3U as fetched.
const netcastCacheDir = "/data/Netcast/server"

var netcastIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// readNetcastList reads the list Netcast cached for a playlist source and
// keeps it like a list read from an address, so the IPTV tab's picking,
// guide, test and plan work on it. Nothing is fetched from the internet:
// it is the list the TV shows. Such a list is not remembered for "Compare
// again" (it has no address of its own).
func (h *Channels) readNetcastList(w http.ResponseWriter, r *http.Request) {
	h.openNetcastList(w, r, false)
}

// openNetcastList is readNetcastList; with allowFetch a playlist source the
// plugin has not downloaded yet is loaded by DreamHub from the address in the
// source's own file (never one with a login) — what the plugin would do, and
// the same safeguards as any list read from an address.
func (h *Channels) openNetcastList(w http.ResponseWriter, r *http.Request, allowFetch bool) {
	var body struct {
		ID string `json:"id"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || !netcastIDPattern.MatchString(body.ID) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"id"}`)
		return
	}
	rd, ok := h.WC.(webcontrol.BoxFileReader)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, CodeNetcastNoList, "this box has no Netcast lists")
		return
	}
	ctx := r.Context()
	var pl *iptv.Playlist
	var err error
	if allowFetch && body.ID == netcastVavooID {
		pl, err = netcastVavooList(ctx, rd) // NetcastHub only
	} else {
		pl, err = netcastCachedList(ctx, rd, body.ID)
	}
	var view netcastView
	var haveView bool
	fetched := false
	if allowFetch && errors.Is(err, webcontrol.ErrNotFound) {
		if v, verr := readNetcast(ctx, rd); verr == nil {
			view, haveView = v, true
			if src, ok := netcastSourceByID(v, body.ID); ok && src.Fetch != "" && strings.HasPrefix(src.MediaType, "playlist") {
				var raw []byte
				if raw, err = h.fetcher().Fetch(ctx, src.Fetch); err == nil {
					pl, err = parseNetcastPlaylist(body.ID, raw)
					fetched = err == nil
				}
			}
		}
	}
	var fe *iptv.FetchError
	switch {
	case errors.Is(err, errNetcastUnsupported):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, CodeNetcastUnsupported, "DreamHub cannot list the channels of this source")
		return
	case errors.Is(err, webcontrol.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, CodeNetcastNoList, "Netcast has not downloaded that list yet")
		return
	case errors.Is(err, webcontrol.ErrTooLarge):
		writeFetchError(w, r, &iptv.FetchError{Kind: iptv.KindTooLarge})
		return
	case errors.As(err, &fe):
		writeFetchError(w, r, err)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	name := body.ID
	if !haveView {
		if v, err := readNetcast(ctx, rd); err == nil {
			view, haveView = v, true
		}
	}
	if haveView {
		if s, ok := netcastSourceByID(view, body.ID); ok && s.Name != "" {
			name = s.Name
		}
	}
	user, _ := httpx.User(ctx)
	s := &iptvSession{token: randomHex(16), userID: user.ID, name: "Netcast: " + name, fromFile: true, read: h.now(), pl: pl}
	h.keepIPTV(s)
	out := iptvListView(s)
	out["fetched"] = fetched
	httpx.WriteJSON(w, http.StatusOK, out)
}

// netcastSourceByID finds a source of the "server" files by its id.
func netcastSourceByID(v netcastView, id string) (netcast.Source, bool) {
	for _, f := range v.Files {
		if f.Kind != "server" {
			continue
		}
		for _, s := range f.Sources {
			if s.ID == id {
				return s, true
			}
		}
	}
	return netcast.Source{}, false
}
