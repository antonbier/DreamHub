package hubs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Copying the channel list from one box to another (docs/MULTIDEVICE_PLAN.md,
// M8): the bouquets of box A, applied to box B as one plan. Service
// references differ per box, so channels are matched by their reference
// first (same satellite, same service) and by name second; a channel B
// doesn't know is left out and listed in the preview. A bouquet B has under
// the same name and kind is made to look like A's; one B lacks is created.
// Nothing else on B is touched: its other bouquets stay as they are.
//
// Applying is ChannelHub's usual way (a backup of B's whole channel list
// first, each bouquet read back and compared, the backup restored on any
// difference), so it is as safe as editing one bouquet by hand.

// Error codes.
const (
	CodeSyncSameBox = "sync_same_box" // source and target are one box
)

type syncRequest struct {
	From string `json:"from"` // the source box
	// Bouquets: the source bouquets to copy (their references on the source
	// box); empty means all of them.
	Bouquets []string `json:"bouquets"`
}

// SyncBouquet is one source bouquet as the preview shows it.
type SyncBouquet struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Action: "create" (the target has none by that name), "update" or
	// "unchanged".
	Action string `json:"action"`
	// Channels is how many channels the bouquet will have; Changes the
	// steps it takes to get there.
	Channels int `json:"channels"`
	Changes  int `json:"changes"`
	// Missing are the source's channels the target box doesn't know.
	Missing []string `json:"missing,omitempty"`
}

// syncItem is the work for one bouquet, worked out at planning time.
type syncItem struct {
	SyncBouquet
	srcRef string
	ref    string // the target's bouquet, "" while it doesn't exist
	draft  []DraftEntry
}

// normName makes channel names comparable: case, spaces and punctuation
// don't matter ("SAT.1 HD" = "sat 1 hd"); "HD" does, it is a different
// channel.
func normName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// targetChannels indexes every channel a box knows by reference and name.
type targetChannels struct {
	byKey  map[string]webcontrol.Service
	byName map[string][]webcontrol.Service
}

func (h *Channels) knownChannels(ctx context.Context) (targetChannels, error) {
	tc := targetChannels{byKey: map[string]webcontrol.Service{}, byName: map[string][]webcontrol.Service{}}
	for _, root := range []string{webcontrol.TVAll, webcontrol.RadioAll} {
		list, err := h.WC.Services(ctx, root)
		if err != nil {
			return tc, err
		}
		for _, s := range list {
			if webcontrol.IsMarker(s.ServiceReference) || unnamedService(s.ServiceName) {
				continue
			}
			tc.byKey[channelKey(s.ServiceReference)] = s
			n := normName(s.ServiceName)
			tc.byName[n] = append(tc.byName[n], s)
		}
	}
	return tc, nil
}

// find is the target's channel for a source entry.
func (tc targetChannels) find(src webcontrol.Service) (webcontrol.Service, bool) {
	if s, ok := tc.byKey[channelKey(src.ServiceReference)]; ok {
		return s, true
	}
	candidates := tc.byName[normName(src.ServiceName)]
	if len(candidates) == 0 {
		return webcontrol.Service{}, false
	}
	// Several with one name: the same quality, else the first.
	for _, c := range candidates {
		if webcontrol.Quality(c.ServiceReference) == webcontrol.Quality(src.ServiceReference) {
			return c, true
		}
	}
	return candidates[0], true
}

// syncState is what a plan is made from: the chosen source bouquets and
// the target's list, bouquets and channels. Its fingerprint says whether
// either box changed since the preview.
type syncState struct {
	src      []BouquetSummary
	srcLists map[string][]webcontrol.Service
	dst      []BouquetSummary
	dstLists map[string][]webcontrol.Service
	known    targetChannels
}

func (h *Channels) readSyncState(ctx, srcCtx context.Context, only []string) (syncState, error) {
	st := syncState{srcLists: map[string][]webcontrol.Service{}, dstLists: map[string][]webcontrol.Service{}}
	all, err := h.allBouquets(srcCtx)
	if err != nil {
		return st, err
	}
	want := map[string]bool{}
	for _, r := range only {
		want[r] = true
	}
	for _, b := range all {
		if len(want) > 0 && !want[b.Ref] {
			continue
		}
		list, err := h.WC.Services(srcCtx, b.Ref)
		if err != nil {
			return st, err
		}
		st.src = append(st.src, b)
		st.srcLists[b.Ref] = list
	}
	if st.dst, err = h.allBouquets(ctx); err != nil {
		return st, err
	}
	for _, b := range st.dst {
		list, err := h.WC.Services(ctx, b.Ref)
		if err != nil {
			return st, err
		}
		st.dstLists[b.Ref] = list
	}
	st.known, err = h.knownChannels(ctx)
	return st, err
}

func (st syncState) fingerprint() string {
	sum := sha256.New()
	for _, b := range st.src {
		fmt.Fprintf(sum, "s\x00%s\x00%s\x01%s\x02", b.Ref, b.Name, listFingerprint(st.srcLists[b.Ref]))
	}
	for _, b := range st.dst {
		fmt.Fprintf(sum, "d\x00%s\x00%s\x01%s\x02", b.Ref, b.Name, listFingerprint(st.dstLists[b.Ref]))
	}
	fmt.Fprintf(sum, "k%d", len(st.known.byKey))
	return hex.EncodeToString(sum.Sum(nil))
}

// items works out, per source bouquet, what the target needs.
func (st syncState) items() []syncItem {
	var out []syncItem
	for _, sb := range st.src {
		it := syncItem{SyncBouquet: SyncBouquet{Name: sb.Name, Kind: sb.Kind}, srcRef: sb.Ref}
		var existing []webcontrol.Service
		for _, db := range st.dst {
			if db.Kind == sb.Kind && normName(db.Name) == normName(sb.Name) {
				it.ref, existing = db.Ref, st.dstLists[db.Ref]
				break
			}
		}
		// A heading already in the target keeps its reference.
		headings := map[string]string{}
		for _, s := range existing {
			if webcontrol.IsMarker(s.ServiceReference) {
				headings[normName(s.ServiceName)] = s.ServiceReference
			}
		}
		seen := map[string]bool{}
		for _, e := range st.srcLists[sb.Ref] {
			if webcontrol.IsMarker(e.ServiceReference) {
				d := DraftEntry{Name: e.ServiceName, Marker: true}
				if ref, ok := headings[normName(e.ServiceName)]; ok {
					d.Ref = ref
					delete(headings, normName(e.ServiceName))
				}
				it.draft = append(it.draft, d)
				continue
			}
			found, ok := st.known.find(e)
			switch {
			case !ok:
				it.Missing = append(it.Missing, e.ServiceName)
			case seen[channelKey(found.ServiceReference)]:
				// a channel is in a bouquet once
			default:
				seen[channelKey(found.ServiceReference)] = true
				it.draft = append(it.draft, DraftEntry{Ref: found.ServiceReference, Name: found.ServiceName})
			}
		}
		it.Channels = 0
		for _, d := range it.draft {
			if !d.Marker {
				it.Channels++
			}
		}
		switch {
		case it.ref == "":
			it.Action, it.Changes = "create", len(it.draft)
		default:
			// Entries of the target keep the reference the box gave them
			// (a name baked into it by an earlier add): match on the key.
			resolved := make([]DraftEntry, len(it.draft))
			copy(resolved, it.draft)
			for i, d := range resolved {
				if d.Ref == "" || d.Marker {
					continue
				}
				for _, s := range existing {
					if channelKey(s.ServiceReference) == channelKey(d.Ref) {
						resolved[i].Ref = s.ServiceReference
						break
					}
				}
			}
			it.draft = resolved
			diff := diffBouquet(existing, it.draft)
			it.Changes = len(diff.changes)
			it.Action = "update"
			if diff.empty() {
				it.Action = "unchanged"
			}
		}
		out = append(out, it)
	}
	return out
}

func (h *Channels) planSync(w http.ResponseWriter, r *http.Request) {
	editor, ok := h.requireEditor(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var req syncRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || req.From == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"from": <box id>, "bouquets": [<source bouquet refs>]}`)
		return
	}
	if req.From == box.Key(ctx) {
		httpx.WriteError(w, r, http.StatusConflict, CodeSyncSameBox, "the channel list is already on that box")
		return
	}
	known := false
	if h.Boxes != nil {
		for _, id := range h.Boxes() {
			known = known || id == req.From
		}
	}
	if !known {
		httpx.WriteError(w, r, http.StatusNotFound, "box_not_found", "no such box")
		return
	}
	user, _ := httpx.User(ctx)
	if !user.MayUseBox(req.From) {
		httpx.WriteError(w, r, http.StatusForbidden, "box_forbidden", "this profile may not use that box")
		return
	}
	srcCtx := box.With(ctx, req.From)
	st, err := h.readSyncState(ctx, srcCtx, req.Bouquets)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	items := st.items()
	var changes []plan.Change
	var bouquets []SyncBouquet
	todo := 0
	for _, it := range items {
		bouquets = append(bouquets, it.SyncBouquet)
		switch it.Action {
		case "create":
			changes = append(changes, plan.Change{Type: "bouquet.create", After: ChannelChange{Name: it.Name}})
		case "update":
			changes = append(changes, plan.Change{Type: "bouquet.sync", After: ChannelChange{Name: it.Name}})
		default:
			continue
		}
		todo++
	}
	if todo == 0 {
		httpx.WriteError(w, r, http.StatusConflict, CodeChannelsUnchanged, "the bouquets are the same on both boxes")
		return
	}
	summary := fmt.Sprintf("Copied %d bouquet(s) from box %q", todo, req.From)
	pl := h.Plans.Create(ctx, user, plan.Draft{
		Action:      "channels.sync",
		Changes:     changes,
		Summary:     summary,
		Detail:      map[string]any{"from": req.From, "bouquets": bouquets},
		Fingerprint: st.fingerprint(),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.readSyncState(ctx, box.With(ctx, req.From), req.Bouquets)
			if err != nil {
				return "", err
			}
			return now.fingerprint(), nil
		},
		Execute: func(ctx context.Context) error {
			return h.withBackup(ctx, editor, user, summary, func(ctx context.Context) error {
				return h.applySync(ctx, editor, items)
			})
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "bouquets": bouquets})
}

// applySync creates what is missing, then brings each bouquet in line the
// way a hand edit does (and checks the result the same way).
func (h *Channels) applySync(ctx context.Context, editor webcontrol.BouquetEditor, items []syncItem) error {
	for _, it := range items {
		if it.Action == "unchanged" {
			continue
		}
		radio := it.Kind == "radio"
		ref, current := it.ref, []webcontrol.Service(nil)
		if it.Action == "create" {
			before, err := h.allBouquets(ctx)
			if err != nil {
				return err
			}
			if err := editor.AddBouquet(ctx, it.Name, radio); err != nil {
				return err
			}
			after, err := h.allBouquets(ctx)
			if err != nil {
				return err
			}
			known := map[string]bool{}
			for _, b := range before {
				known[b.Ref] = true
			}
			ref = ""
			for _, b := range after {
				if !known[b.Ref] && b.Kind == it.Kind {
					ref = b.Ref
				}
			}
			if ref == "" {
				return fmt.Errorf("channels: bouquet %q didn't appear", it.Name)
			}
		} else {
			var err error
			if current, err = h.WC.Services(ctx, ref); err != nil {
				return err
			}
		}
		d := bouquetDraft{Ref: ref, Kind: it.Kind, Entries: it.draft}
		if err := h.applyBouquet(ctx, editor, d, current, diffBouquet(current, it.draft)); err != nil {
			return err
		}
	}
	return nil
}
