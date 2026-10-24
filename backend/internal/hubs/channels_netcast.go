package hubs

import (
	"context"
	"errors"
	"net/http"
	"path"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/netcast"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Netcast's source files, read only (docs/NETCAST_PLAN.md, N3). DreamHub
// lists what the Gemini Netcast plugin has configured on the box; it writes
// nothing — that needs the plugin's write route and comes with box settings
// S3. Logins in a file never leave the backend.

type netcastView struct {
	// Available is false when the box serves no files or has no Netcast file.
	Available bool           `json:"available"`
	Files     []netcast.File `json:"files"`
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
