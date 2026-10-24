package hubs

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Remembered playlists and "Compare again" (docs/NETCAST_PLAN.md, N1).
//
// DreamHub keeps only the source: the address of the list, the bouquet it
// fills, which channels it put there (as hashes) and a key for every channel
// the list had at the last look. The bouquet on the box stays the truth. A
// comparison reads the list again and the bouquet as it is now and offers,
// never does by itself:
//
//	update — a channel of the bouquet whose address changed in the list (ticked)
//	gone   — a channel of the bouquet the list no longer has (not ticked)
//	new    — a channel the list gained since the last look (not ticked)
//
// What the person added or moved in the bouquet by hand is left alone.

const CodeIPTVBouquetGone = "iptv_bouquet_gone"

// iptvCompare is the result of a comparison, kept with the fresh list.
type iptvCompare struct {
	sourceID   int64
	bouquetRef string
	kind       string
	items      []iptvCompareItem
	unchanged  int
}

type iptvCompareItem struct {
	Action  string // "new", "update", "gone"
	Fresh   int    // index in the fresh list; -1 for "gone"
	OldRef  string // the bouquet entry (update, gone)
	OldName string
	Name    string
}

// iptvKey identifies a channel across address changes: its tvg-id, else its
// name; both are the list's own words.
func iptvKey(e iptv.Entry) uint64 {
	key := ""
	if id := strings.ToLower(strings.TrimSpace(e.TvgID)); id != "" {
		key = "t:" + id
	} else if n := normName(e.Name); n != "" {
		key = "n:" + n
	} else {
		key = "u:" + iptv.URLPart(iptv.Ref(e.URL))
	}
	sum := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(sum[:8])
}

func iptvIdentHex(k uint64) string { return fmt.Sprintf("%016x", k) }

// iptvURLHash hashes the address part of a 4097 reference: what is stored
// instead of the address, which may hold a login.
func iptvURLHash(part string) string {
	sum := sha256.Sum256([]byte(part))
	return hex.EncodeToString(sum[:12])
}

func iptvMember(e iptv.Entry) store.IPTVMember {
	return store.IPTVMember{URLHash: iptvURLHash(iptv.URLPart(iptv.Ref(e.URL))), Ident: iptvIdentHex(iptvKey(e)), Name: e.Name}
}

func iptvSeenKeys(pl *iptv.Playlist) []uint64 {
	keys := make([]uint64, 0, len(pl.Entries))
	for _, e := range pl.Entries {
		keys = append(keys, iptvKey(e))
	}
	return keys
}

// rememberIPTV keeps a list that was just put into a bouquet. A failure only
// costs the comparison later, so it is logged and the plan still succeeds.
func (h *Channels) rememberIPTV(ctx context.Context, sess *iptvSession, bouquetRef, bouquetName, kind string, added []iptv.Entry) {
	if sess.url == "" || sess.fromFile || bouquetRef == "" {
		return
	}
	members := make([]store.IPTVMember, 0, len(added))
	for _, e := range added {
		members = append(members, iptvMember(e))
	}
	name := sess.name
	if name == "" {
		name = iptv.Host(sess.url)
	}
	_, err := h.Store.SaveIPTVSource(ctx, store.IPTVSource{
		Name: name, URL: sess.url, DisplayURL: sess.display, BouquetRef: bouquetRef, BouquetName: bouquetName, Kind: kind,
		Seen: iptvSeenKeys(sess.pl),
	}, members)
	if err != nil {
		slog.Warn("could not remember the playlist", "err", err)
	}
}

// --- the list of remembered playlists ---------------------------------------------

type iptvSourceView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Source     string `json:"source"` // without login and query
	Bouquet    string `json:"bouquet"`
	BouquetRef string `json:"bouquetRef"`
	Kind       string `json:"kind"`
	ComparedAt string `json:"comparedAt,omitempty"`
	Gone       bool   `json:"gone,omitempty"` // the bouquet is no longer on the box
}

func viewIPTVSource(x store.IPTVSource, bouquets []BouquetSummary, known bool) iptvSourceView {
	v := iptvSourceView{ID: x.ID, Name: x.Name, Source: x.DisplayURL, Bouquet: x.BouquetName, BouquetRef: x.BouquetRef, Kind: x.Kind}
	if !x.ComparedAt.IsZero() {
		v.ComparedAt = x.ComparedAt.UTC().Format(time.RFC3339)
	}
	if known {
		v.Gone = true
		for _, b := range bouquets {
			if b.Ref == x.BouquetRef {
				v.Gone, v.Bouquet = false, b.Name
			}
		}
	}
	return v
}

func (h *Channels) iptvSources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := h.Store.IPTVSources(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the remembered lists")
		return
	}
	bouquets, berr := h.allBouquets(ctx) // the box may be off: then "gone" is not known
	out := make([]iptvSourceView, 0, len(list))
	for _, x := range list {
		out = append(out, viewIPTVSource(x, bouquets, berr == nil))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sources": out})
}

func (h *Channels) sourceFromPath(w http.ResponseWriter, r *http.Request) (store.IPTVSource, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such list")
		return store.IPTVSource{}, false
	}
	src, err := h.Store.IPTVSource(r.Context(), id)
	if errors.Is(err, store.ErrIPTVSourceNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such list")
		return store.IPTVSource{}, false
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the remembered list")
		return store.IPTVSource{}, false
	}
	return src, true
}

func (h *Channels) forgetIPTV(w http.ResponseWriter, r *http.Request) {
	src, ok := h.sourceFromPath(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteIPTVSource(r.Context(), src.ID); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not forget the list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- comparing ----------------------------------------------------------------------

type iptvCompareItemView struct {
	ID       int    `json:"id"`
	Action   string `json:"action"`
	Name     string `json:"name"`
	OldName  string `json:"oldName,omitempty"` // as it is in the bouquet, when that differs
	Group    string `json:"group,omitempty"`
	Host     string `json:"host,omitempty"`
	HasLogin bool   `json:"hasLogin,omitempty"`
	HasLogo  bool   `json:"hasLogo,omitempty"`
}

func (h *Channels) compareIPTV(w http.ResponseWriter, r *http.Request) {
	src, ok := h.sourceFromPath(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	raw, err := h.fetcher().Fetch(ctx, src.URL)
	if err != nil {
		writeFetchError(w, r, err)
		return
	}
	pl, err := iptv.Parse(strings.NewReader(string(raw)))
	if err != nil {
		writeFetchError(w, r, err)
		return
	}
	bouquets, err := h.allBouquets(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	found := false
	for _, b := range bouquets {
		found = found || b.Ref == src.BouquetRef
	}
	if !found {
		httpx.WriteError(w, r, http.StatusConflict, CodeIPTVBouquetGone, "the bouquet of this list is no longer on the box")
		return
	}
	current, err := h.WC.Services(ctx, src.BouquetRef)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	members, err := h.Store.IPTVMembers(ctx, src.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read the remembered list")
		return
	}
	cmp := compareIPTVList(pl, current, members, src.Seen)
	cmp.sourceID, cmp.bouquetRef, cmp.kind = src.ID, src.BouquetRef, src.Kind
	_ = h.Store.MarkIPTVCompared(ctx, src.ID)

	s := &iptvSession{token: randomHex(16), userID: user.ID, name: src.Name, display: src.DisplayURL, url: src.URL, read: h.now(), pl: pl, compare: cmp}
	h.keepIPTV(s)
	items := make([]iptvCompareItemView, 0, len(cmp.items))
	for i, it := range cmp.items {
		v := iptvCompareItemView{ID: i, Action: it.Action, Name: it.Name}
		if it.OldName != "" && it.OldName != it.Name {
			v.OldName = it.OldName
		}
		if it.Fresh >= 0 {
			e := pl.Entries[it.Fresh]
			v.Group, v.Host, v.HasLogin, v.HasLogo = e.Group, iptv.Host(e.URL), iptv.HasLogin(e.URL), e.Logo != ""
		}
		items = append(items, v)
	}
	view := viewIPTVSource(src, bouquets, true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"token": s.token, "source": view, "items": items, "unchanged": cmp.unchanged, "listSize": len(pl.Entries), "truncated": pl.Truncated})
}

// compareIPTVList works out what differs. current is the bouquet as it is,
// members what was put there from this list, seen the list's keys at the last
// look.
func compareIPTVList(pl *iptv.Playlist, current []webcontrol.Service, members []store.IPTVMember, seen []uint64) *iptvCompare {
	cmp := &iptvCompare{}
	memberByHash := map[string]store.IPTVMember{}
	for _, m := range members {
		memberByHash[m.URLHash] = m
	}
	seenSet := map[uint64]bool{}
	for _, k := range seen {
		seenSet[k] = true
	}
	// the list, without a second copy of the same address
	freshByPart := map[string]int{}
	freshByIdent := map[string][]int{}
	order := make([]int, 0, len(pl.Entries))
	for i, e := range pl.Entries {
		part := iptv.URLPart(iptv.Ref(e.URL))
		if _, dup := freshByPart[part]; dup {
			continue
		}
		freshByPart[part] = i
		order = append(order, i)
		id := iptvIdentHex(iptvKey(e))
		freshByIdent[id] = append(freshByIdent[id], i)
	}
	inBouquet := map[string]bool{}
	for _, s := range current {
		if p := iptv.URLPart(s.ServiceReference); p != "" {
			inBouquet[p] = true
		}
	}
	used := map[int]bool{}

	type pending struct {
		svc webcontrol.Service
		m   store.IPTVMember
	}
	var open []pending
	for _, s := range current {
		part := iptv.URLPart(s.ServiceReference)
		if part == "" {
			continue
		}
		if i, ok := freshByPart[part]; ok {
			used[i] = true // still in the list; ours or added by hand, either way unchanged
			if _, ours := memberByHash[iptvURLHash(part)]; ours {
				cmp.unchanged++
			}
			continue
		}
		if m, ours := memberByHash[iptvURLHash(part)]; ours {
			open = append(open, pending{s, m})
		}
	}
	var updates, gone []iptvCompareItem
	for _, p := range open {
		match := -1
		for _, i := range freshByIdent[p.m.Ident] {
			if !used[i] && !inBouquet[iptv.URLPart(iptv.Ref(pl.Entries[i].URL))] {
				match = i
				break
			}
		}
		if match >= 0 {
			used[match] = true
			updates = append(updates, iptvCompareItem{Action: "update", Fresh: match, OldRef: p.svc.ServiceReference, OldName: p.svc.ServiceName, Name: pl.Entries[match].Name})
		} else {
			gone = append(gone, iptvCompareItem{Action: "gone", Fresh: -1, OldRef: p.svc.ServiceReference, OldName: p.svc.ServiceName, Name: p.svc.ServiceName})
		}
	}
	var fresh []iptvCompareItem
	for _, i := range order {
		e := pl.Entries[i]
		if used[i] || inBouquet[iptv.URLPart(iptv.Ref(e.URL))] || seenSet[iptvKey(e)] {
			continue
		}
		fresh = append(fresh, iptvCompareItem{Action: "new", Fresh: i, Name: e.Name})
	}
	cmp.items = append(append(append(cmp.items, updates...), gone...), fresh...)
	return cmp
}

// --- the plan of a comparison ---------------------------------------------------------

type iptvComparePlanRequest struct {
	Token string `json:"token"`
	IDs   []int  `json:"ids"` // comparison items to carry out
	// Logos fetches the logos of the new channels chosen.
	Logos bool `json:"logos"`
}

func (h *Channels) planIPTVCompare(w http.ResponseWriter, r *http.Request) {
	editor, ok := h.requireEditor(w, r)
	if !ok {
		return
	}
	src, ok := h.sourceFromPath(w, r)
	if !ok {
		return
	}
	var req iptvComparePlanRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"token", "ids": [...]}`)
		return
	}
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	sess, found := h.iptvSession(req.Token, user.ID)
	if !found || sess.compare == nil || sess.compare.sourceID != src.ID {
		httpx.WriteError(w, r, http.StatusGone, CodeIPTVSessionGone, "the comparison was made too long ago; compare again")
		return
	}
	cmp := sess.compare
	chosen := map[int]bool{}
	for _, id := range req.IDs {
		if id < 0 || id >= len(cmp.items) {
			httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was planned", map[string]httpx.FieldError{"ids": {Code: "invalid"}})
			return
		}
		chosen[id] = true
	}
	if len(chosen) == 0 {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was planned", map[string]httpx.FieldError{"ids": {Code: "required"}})
		return
	}
	current, err := h.WC.Services(ctx, cmp.bouquetRef)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}

	// The bouquet as it should be: updated entries in place, gone ones
	// removed, new ones at the end. What is not mentioned stays as it is.
	draft := make([]DraftEntry, 0, len(current)+len(chosen))
	replace := map[string]DraftEntry{}
	drop := map[string]bool{}
	var appended []DraftEntry
	remembered, _ := h.Store.IPTVTwins(ctx)
	var logoJobs []iptvLogoJob
	type moved struct{ from, to string }
	var moves []moved
	newRef := map[int]string{} // a new channel's reference: with its remembered guide, if it has one
	var removeHashes []string
	var addMembers []store.IPTVMember
	notTaken := map[uint64]bool{} // new channels left unticked stay "new" for the next comparison
	for i, it := range cmp.items {
		if !chosen[i] {
			if it.Action == "new" {
				notTaken[iptvKey(sess.pl.Entries[it.Fresh])] = true
			}
			continue
		}
		switch it.Action {
		case "update":
			e := sess.pl.Entries[it.Fresh]
			// the channel keeps the identity (and so the guide) it has
			ref := iptv.WithURL(it.OldRef, e.URL)
			if ref == "" {
				ref = iptv.Ref(e.URL)
			}
			replace[it.OldRef] = DraftEntry{Ref: ref, Name: it.OldName}
			moves = append(moves, moved{it.OldRef, ref + ":" + it.OldName})
			removeHashes = append(removeHashes, iptvURLHash(iptv.URLPart(it.OldRef)))
			addMembers = append(addMembers, iptvMember(e))
		case "gone":
			drop[it.OldRef] = true
			removeHashes = append(removeHashes, iptvURLHash(iptv.URLPart(it.OldRef)))
		case "new":
			e := sess.pl.Entries[it.Fresh]
			name := strings.TrimSpace(e.Name)
			if utf8.RuneCountInString(name) > 100 {
				name = string([]rune(name)[:100])
			}
			ref := iptv.Ref(e.URL)
			if t, ok := remembered[iptvIdentHex(iptvKey(e))]; ok {
				if tr, ok := iptv.TwinRef(t.ServiceRef, e.URL); ok {
					ref = tr
				}
			}
			newRef[it.Fresh] = ref
			appended = append(appended, DraftEntry{Ref: ref, Name: name})
			if req.Logos {
				if j, ok := iptvLogoJobFor(e, ref+":"+name); ok {
					logoJobs = append(logoJobs, j)
				}
			}
			addMembers = append(addMembers, iptvMember(e))
		}
	}
	present := map[string]bool{}
	for _, s := range current {
		present[s.ServiceReference] = true
	}
	for ref := range replace {
		if !present[ref] {
			httpx.WriteError(w, r, http.StatusGone, CodeIPTVSessionGone, "the bouquet changed since the comparison; compare again")
			return
		}
	}
	for ref := range drop {
		if !present[ref] {
			httpx.WriteError(w, r, http.StatusGone, CodeIPTVSessionGone, "the bouquet changed since the comparison; compare again")
			return
		}
	}
	for _, s := range current {
		switch {
		case drop[s.ServiceReference]:
		case replace[s.ServiceReference].Ref != "":
			draft = append(draft, replace[s.ServiceReference])
		default:
			draft = append(draft, DraftEntry{Ref: s.ServiceReference, Name: s.ServiceName, Marker: webcontrol.IsMarker(s.ServiceReference)})
		}
	}
	draft = append(draft, appended...)

	diff := diffBouquet(current, draft)
	if diff.empty() {
		httpx.WriteError(w, r, http.StatusConflict, CodeChannelsUnchanged, "the chosen changes leave the bouquet as it is")
		return
	}
	// The preview says what the person chose, in the person's terms: an
	// address change is one line, not a removal and an addition.
	num := channelNumbers(draft, func(e DraftEntry) bool { return e.Marker })
	var changes []plan.Change
	for i, it := range cmp.items {
		if !chosen[i] {
			continue
		}
		switch it.Action {
		case "update":
			changes = append(changes, plan.Change{Type: "channel.update", Before: ChannelChange{Name: it.OldName}})
		case "gone":
			changes = append(changes, plan.Change{Type: "channel.remove", Before: ChannelChange{Name: it.OldName}})
		case "new":
			to := 0
			for k, d := range draft {
				if d.Ref == newRef[it.Fresh] {
					to = num[k]
				}
			}
			changes = append(changes, plan.Change{Type: "channel.add", After: ChannelChange{Name: it.Name, To: to}})
		}
	}

	bouquetName := src.BouquetName
	if list, err := h.allBouquets(ctx); err == nil {
		for _, b := range list {
			if b.Ref == cmp.bouquetRef {
				bouquetName = b.Name
			}
		}
	}
	summary := fmt.Sprintf("Compared bouquet %q with its list: %d changes", bouquetName, len(changes))
	bref, kind := cmp.bouquetRef, cmp.kind
	seen := make([]uint64, 0, len(sess.pl.Entries))
	for _, e := range sess.pl.Entries {
		if k := iptvKey(e); !notTaken[k] {
			seen = append(seen, k)
		}
	}
	pl := h.Plans.Create(ctx, user, plan.Draft{
		Action:      "channels.iptv.compare",
		Changes:     changes,
		Summary:     summary,
		Detail:      map[string]any{"bouquet": bouquetName, "source": src.Name, "count": len(changes)},
		Fingerprint: listFingerprint(current),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.WC.Services(ctx, bref)
			if err != nil {
				return "", err
			}
			return listFingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			if err := h.withBackup(ctx, editor, user, summary, func(ctx context.Context) error {
				return h.applyBouquet(ctx, editor, bouquetDraft{Ref: bref, Kind: kind, Entries: draft}, current, diff)
			}); err != nil {
				return err
			}
			for _, m := range moves {
				moveLogo(m.from, m.to)
			}
			h.fetchIPTVLogos(logoJobs)
			err := h.Store.UpdateIPTVSource(ctx, src.ID, store.IPTVChange{RemoveHashes: removeHashes, Add: addMembers, Seen: seen, BouquetName: bouquetName})
			if err != nil {
				slog.Warn("could not update the remembered playlist", "err", err)
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "logos": map[string]any{"count": len(logoJobs), "hosts": iptvLogoHosts(logoJobs)}})
}
