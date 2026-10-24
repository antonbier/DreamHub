package hubs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// IPTV from an M3U list into a bouquet (docs/NETCAST_PLAN.md, N1). A list is
// read once (from an address, or uploaded) and kept on the server under a
// token for a while; the screen then works with channel numbers and the
// token, so a stream address — which may carry a login — never travels to
// the browser and never appears in a preview, a plan or a log. Putting the
// chosen channels into a bouquet is an ordinary bouquet plan: preview,
// confirm, a backup of the channel list first, the result read back.

// Error codes of the IPTV routes.
const (
	CodeIPTVUnreachable  = "iptv_unreachable"   // the address gave no answer
	CodeIPTVBlocked      = "iptv_blocked"       // the address leads to this box or a link-local address
	CodeIPTVBadURL       = "iptv_invalid_url"   // not an http(s) address
	CodeIPTVStatus       = "iptv_server_status" // the server answered with an error
	CodeIPTVTooLarge     = "iptv_too_large"
	CodeIPTVNotPlaylist  = "iptv_not_playlist"
	CodeIPTVSessionGone  = "iptv_session_gone" // read the list again
	CodeIPTVNameTaken    = "iptv_bouquet_exists"
	CodeIPTVNothingToAdd = "iptv_nothing_to_add"
)

const (
	iptvSessionTTL    = 20 * time.Minute
	iptvPerUser       = 3 // lists kept per profile
	iptvTotal         = 6 // and in all, a list is a few MB
	iptvMaxPick       = 5000
	iptvMaxUploadSize = 8 << 20
)

// iptvSession is a list read for one profile.
type iptvSession struct {
	token    string
	userID   int64
	name     string // a name for the source: the host of its address, or the file's name
	display  string // the address without login, query and fragment; "" for a file
	fromFile bool
	read     time.Time
	pl       *iptv.Playlist
	// url is the address as given, possibly with a login: kept on the server
	// to remember the list; never sent to the browser.
	url string
	// compare is set for a list read to compare it with its bouquet.
	compare *iptvCompare
}

func (h *Channels) fetcher() *iptv.Fetcher {
	if h.IPTV != nil {
		return h.IPTV
	}
	return iptv.NewFetcher()
}

// keepIPTV stores a read list and drops old ones.
func (h *Channels) keepIPTV(s *iptvSession) {
	h.iptvMu.Lock()
	defer h.iptvMu.Unlock()
	now := h.now()
	live := h.iptvSess[:0]
	for _, o := range h.iptvSess {
		if now.Sub(o.read) < iptvSessionTTL {
			live = append(live, o)
		}
	}
	h.iptvSess = live
	// oldest first: drop the oldest of this profile beyond its share, then
	// the oldest of all beyond the total
	own := 0
	for _, o := range h.iptvSess {
		if o.userID == s.userID {
			own++
		}
	}
	for own >= iptvPerUser {
		for i, o := range h.iptvSess {
			if o.userID == s.userID {
				h.iptvSess = append(h.iptvSess[:i], h.iptvSess[i+1:]...)
				own--
				break
			}
		}
	}
	for len(h.iptvSess) >= iptvTotal {
		h.iptvSess = h.iptvSess[1:]
	}
	h.iptvSess = append(h.iptvSess, s)
}

func (h *Channels) iptvSession(token string, userID int64) (*iptvSession, bool) {
	h.iptvMu.Lock()
	defer h.iptvMu.Unlock()
	for _, s := range h.iptvSess {
		if s.token == token && s.userID == userID && h.now().Sub(s.read) < iptvSessionTTL {
			return s, true
		}
	}
	return nil, false
}

// --- reading a list -----------------------------------------------------------------

// IPTVChannel is one channel of a read list, as the screen sees it: no
// address, only where it points and whether it needs a login.
type IPTVChannel struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	TvgID     string `json:"tvgId,omitempty"`
	Host      string `json:"host"`
	Scheme    string `json:"scheme"`
	Radio     bool   `json:"radio,omitempty"`
	HasLogin  bool   `json:"hasLogin,omitempty"`
	HasLogo   bool   `json:"hasLogo,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"` // the same address as an earlier channel of the list
	// NeedsHeaders: the list asks for a user agent or other headers
	// (#EXTVLCOPT), which the box's player cannot send.
	NeedsHeaders bool `json:"needsHeaders,omitempty"`
}

type iptvGroup struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type iptvSkipped struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func iptvListView(s *iptvSession) map[string]any {
	seen := map[string]bool{}
	groups := map[string]int{}
	out := make([]IPTVChannel, 0, len(s.pl.Entries))
	for i, e := range s.pl.Entries {
		c := IPTVChannel{ID: i, Name: e.Name, Group: e.Group, TvgID: e.TvgID, Host: iptv.Host(e.URL), Scheme: iptv.Scheme(e.URL),
			Radio: e.Radio, HasLogin: iptv.HasLogin(e.URL), HasLogo: e.Logo != "", NeedsHeaders: len(e.Options) > 0}
		key := iptv.Ref(e.URL)
		c.Duplicate = seen[key]
		seen[key] = true
		groups[e.Group]++
		out = append(out, c)
	}
	gl := make([]iptvGroup, 0, len(groups))
	for n, c := range groups {
		gl = append(gl, iptvGroup{n, c})
	}
	sort.Slice(gl, func(i, j int) bool { return strings.ToLower(gl[i].Name) < strings.ToLower(gl[j].Name) })
	skipped := map[string]int{}
	for _, sk := range s.pl.Skipped {
		skipped[sk.Reason]++
	}
	sl := make([]iptvSkipped, 0, len(skipped))
	for r, c := range skipped {
		sl = append(sl, iptvSkipped{r, c})
	}
	sort.Slice(sl, func(i, j int) bool { return sl[i].Reason < sl[j].Reason })
	return map[string]any{
		"token": s.token, "name": s.name, "source": s.display, "fromFile": s.fromFile,
		"channels": out, "groups": gl, "skipped": sl, "truncated": s.pl.Truncated, "limit": iptv.MaxEntries,
	}
}

func writeFetchError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *iptv.FetchError
	if !errors.As(err, &fe) && !errors.Is(err, iptv.ErrNotPlaylist) {
		fe = &iptv.FetchError{Kind: iptv.KindUnreachable}
	}
	if errors.Is(err, iptv.ErrNotPlaylist) {
		fe = &iptv.FetchError{Kind: iptv.KindNotPlaylist}
	}
	switch fe.Kind {
	case iptv.KindInvalidURL:
		httpx.WriteError(w, r, http.StatusBadRequest, CodeIPTVBadURL, "the address must start with http:// or https://")
	case iptv.KindBlocked:
		httpx.WriteError(w, r, http.StatusBadRequest, CodeIPTVBlocked, "that address leads to this box itself")
	case iptv.KindStatus:
		httpx.WriteErrorDetails(w, r, http.StatusBadGateway, CodeIPTVStatus, "the server did not give the list", map[string]any{"status": fe.Status})
	case iptv.KindTooLarge:
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, CodeIPTVTooLarge, "the list is too large")
	case iptv.KindNotPlaylist:
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, CodeIPTVNotPlaylist, "that is not an M3U list")
	default:
		httpx.WriteError(w, r, http.StatusBadGateway, CodeIPTVUnreachable, "the address gave no answer")
	}
}

// readIPTV reads a list from an address.
func (h *Channels) readIPTV(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.URL) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"url"}`)
		return
	}
	raw, err := h.fetcher().Fetch(r.Context(), body.URL)
	if err != nil {
		writeFetchError(w, r, err)
		return
	}
	pl, err := iptv.Parse(strings.NewReader(string(raw)))
	if err != nil {
		writeFetchError(w, r, err)
		return
	}
	user, _ := httpx.User(r.Context())
	s := &iptvSession{token: randomHex(16), userID: user.ID, display: iptv.Display(strings.TrimSpace(body.URL)),
		read: h.now(), pl: pl}
	s.name = iptv.Host(strings.TrimSpace(body.URL))
	s.url = strings.TrimSpace(body.URL)
	h.keepIPTV(s)
	httpx.WriteJSON(w, http.StatusOK, iptvListView(s))
}

// uploadIPTV reads a list the person uploaded; the body is the list itself.
func (h *Channels) uploadIPTV(w http.ResponseWriter, r *http.Request) {
	raw, err := h.fetcher().ReadUpload(http.MaxBytesReader(w, r.Body, iptvMaxUploadSize+1))
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			err = &iptv.FetchError{Kind: iptv.KindTooLarge}
		}
		writeFetchError(w, r, err)
		return
	}
	pl, err := iptv.Parse(strings.NewReader(string(raw)))
	if err != nil {
		writeFetchError(w, r, err)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if utf8.RuneCountInString(name) > 80 {
		name = string([]rune(name)[:80])
	}
	user, _ := httpx.User(r.Context())
	s := &iptvSession{token: randomHex(16), userID: user.ID, name: name, fromFile: true, read: h.now(), pl: pl}
	h.keepIPTV(s)
	httpx.WriteJSON(w, http.StatusOK, iptvListView(s))
}

// --- the plan --------------------------------------------------------------------------

type iptvPlanRequest struct {
	Token string `json:"token"`
	IDs   []int  `json:"ids"`
	Kind  string `json:"kind"` // "tv" or "radio", for a new bouquet
	// Bouquet is an existing bouquet's reference; NewName makes a new bouquet.
	// Exactly one is given.
	Bouquet string `json:"bouquet"`
	NewName string `json:"newName"`
	// Remember keeps the list's address so it can be compared with the
	// bouquet later; ignored for an uploaded file.
	Remember bool `json:"remember"`
	// Twins gives channels of the list the guide of a channel of the box:
	// Service is that channel's reference (internal/hubs/channels_iptv_twins.go).
	Twins []iptvTwinPick `json:"twins"`
	// Logos fetches the logos the list gives for the chosen channels, from the
	// list's own servers, once the change is confirmed.
	Logos bool `json:"logos"`
}

type iptvTwinPick struct {
	ID      int    `json:"id"`
	Service string `json:"service"`
}

// IPTVSkip is a chosen channel that is not added, and why.
type IPTVSkip struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"` // "in_bouquet", "duplicate"
}

func (h *Channels) createBouquet(ctx context.Context, editor webcontrol.BouquetEditor, name, kind string) (string, error) {
	before, err := h.allBouquets(ctx)
	if err != nil {
		return "", err
	}
	if err := editor.AddBouquet(ctx, name, kind == "radio"); err != nil {
		return "", err
	}
	after, err := h.allBouquets(ctx)
	if err != nil {
		return "", err
	}
	known := map[string]bool{}
	for _, b := range before {
		known[b.Ref] = true
	}
	for _, b := range after {
		if !known[b.Ref] && b.Kind == kind {
			return b.Ref, nil
		}
	}
	return "", fmt.Errorf("channels: bouquet %q didn't appear", name)
}

func bouquetsFingerprint(l []BouquetSummary) string {
	var s []webcontrol.Service
	for _, b := range l {
		s = append(s, webcontrol.Service{ServiceReference: b.Ref, ServiceName: b.Name})
	}
	return listFingerprint(s)
}

func (h *Channels) planIPTV(w http.ResponseWriter, r *http.Request) {
	editor, ok := h.requireEditor(w, r)
	if !ok {
		return
	}
	var req iptvPlanRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"token", "ids": [...], "bouquet" | "newName", "kind"}`)
		return
	}
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	sess, found := h.iptvSession(req.Token, user.ID)
	if !found {
		httpx.WriteError(w, r, http.StatusGone, CodeIPTVSessionGone, "the list was read too long ago; read it again")
		return
	}
	req.NewName = strings.TrimSpace(req.NewName)
	errs := map[string]httpx.FieldError{}
	if (req.Bouquet == "") == (req.NewName == "") {
		errs["bouquet"] = httpx.FieldError{Code: "required"}
	}
	if req.NewName != "" && (utf8.RuneCountInString(req.NewName) > 60) {
		errs["newName"] = httpx.FieldError{Code: "invalid"}
	}
	if req.NewName != "" && req.Kind != "tv" && req.Kind != "radio" {
		errs["kind"] = httpx.FieldError{Code: "invalid"}
	}
	if len(req.IDs) == 0 || len(req.IDs) > iptvMaxPick {
		errs["ids"] = httpx.FieldError{Code: "invalid"}
	}
	picked := map[int]bool{}
	for _, id := range req.IDs {
		if id < 0 || id >= len(sess.pl.Entries) {
			errs["ids"] = httpx.FieldError{Code: "invalid"}
			break
		}
		picked[id] = true
	}
	twinOf := map[int]twinService{}
	if len(req.Twins) > 0 && len(errs) == 0 {
		_, byKey, terr := h.twinServices(ctx)
		if terr != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
			return
		}
		for _, tw := range req.Twins {
			svc, known := byKey[channelKey(tw.Service)]
			if !picked[tw.ID] || !known {
				errs["twins"] = httpx.FieldError{Code: "invalid"}
				break
			}
			twinOf[tw.ID] = svc
		}
	}
	if len(errs) > 0 {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was planned", errs)
		return
	}

	bouquets, err := h.allBouquets(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	var target *BouquetSummary
	kind := req.Kind
	bouquetName := req.NewName
	var current []webcontrol.Service
	if req.Bouquet != "" {
		for i := range bouquets {
			if bouquets[i].Ref == req.Bouquet {
				target = &bouquets[i]
			}
		}
		if target == nil {
			httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was planned", map[string]httpx.FieldError{"bouquet": {Code: "not_found"}})
			return
		}
		kind, bouquetName = target.Kind, target.Name
		if current, err = h.WC.Services(ctx, target.Ref); err != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
			return
		}
	} else if hasBouquetNamed(bouquets, webcontrol.BouquetDisplayName(req.NewName, kind == "radio")) {
		httpx.WriteError(w, r, http.StatusConflict, CodeIPTVNameTaken, "a bouquet with that name exists; choose it as the target instead")
		return
	}

	// The chosen channels in the list's order; an address already in the
	// bouquet, or chosen twice, is left out.
	have := map[string]bool{}
	for _, s := range current {
		if u := iptv.URLPart(s.ServiceReference); u != "" {
			have[u] = true
		}
	}
	ids := make([]int, 0, len(picked))
	for id := range picked {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var add []DraftEntry
	var addedEntries []iptv.Entry
	usedTwins := map[string]store.IPTVTwin{}
	var logoJobs []iptvLogoJob
	var skipped []IPTVSkip
	for _, id := range ids {
		e := sess.pl.Entries[id]
		ref := iptv.Ref(e.URL)
		twin, hasTwin := twinOf[id]
		if hasTwin {
			if tr, ok := iptv.TwinRef(twin.ref, e.URL); ok {
				ref = tr
			} else {
				hasTwin = false
			}
		}
		name := strings.TrimSpace(e.Name)
		if utf8.RuneCountInString(name) > 100 {
			name = string([]rune(name)[:100])
		}
		switch {
		case have[iptv.URLPart(ref)]:
			skipped = append(skipped, IPTVSkip{ID: id, Name: e.Name, Reason: "in_bouquet"})
		default:
			have[iptv.URLPart(ref)] = true
			add = append(add, DraftEntry{Ref: ref, Name: name})
			addedEntries = append(addedEntries, e)
			if req.Logos {
				if j, ok := iptvLogoJobFor(e, namedRef(ref, name)); ok {
					logoJobs = append(logoJobs, j)
				}
			}
			if hasTwin {
				usedTwins[iptvIdentHex(iptvKey(e))] = store.IPTVTwin{ServiceRef: twin.ref, ServiceName: twin.name}
			}
		}
	}
	if len(add) == 0 {
		httpx.WriteErrorDetails(w, r, http.StatusConflict, CodeIPTVNothingToAdd, "every chosen channel is in the bouquet already", map[string]any{"skipped": skipped})
		return
	}

	draft := make([]DraftEntry, 0, len(current)+len(add))
	for _, s := range current {
		draft = append(draft, DraftEntry{Ref: s.ServiceReference, Name: s.ServiceName, Marker: webcontrol.IsMarker(s.ServiceReference)})
	}
	draft = append(draft, add...)
	diff := diffBouquet(current, draft)
	var changes []plan.Change
	if target == nil {
		changes = append(changes, plan.Change{Type: "bouquet.create", After: ChannelChange{Name: webcontrol.BouquetDisplayName(bouquetName, kind == "radio")}})
	}
	changes = append(changes, diff.changes...)

	summary := fmt.Sprintf("Added %d internet channel(s) to bouquet %q", len(add), bouquetName)
	d := plan.Draft{
		Action:  "channels.iptv",
		Changes: changes,
		Summary: summary,
		Detail:  map[string]any{"bouquet": bouquetName, "new": target == nil, "count": len(add), "source": sess.name},
	}
	if target != nil {
		bref := target.Ref
		d.Fingerprint = listFingerprint(current)
		d.Current = func(ctx context.Context) (string, error) {
			now, err := h.WC.Services(ctx, bref)
			if err != nil {
				return "", err
			}
			return listFingerprint(now), nil
		}
		d.Execute = func(ctx context.Context) error {
			if err := h.withBackup(ctx, editor, user, summary, func(ctx context.Context) error {
				return h.applyBouquet(ctx, editor, bouquetDraft{Ref: bref, Kind: kind, Entries: draft}, current, diff)
			}); err != nil {
				return err
			}
			if req.Remember {
				h.rememberIPTV(ctx, sess, bref, bouquetName, kind, addedEntries)
			}
			h.rememberTwins(ctx, usedTwins)
			h.fetchIPTVLogos(logoJobs)
			return nil
		}
	} else {
		d.Fingerprint = bouquetsFingerprint(bouquets)
		d.Current = func(ctx context.Context) (string, error) {
			now, err := h.allBouquets(ctx)
			if err != nil {
				return "", err
			}
			return bouquetsFingerprint(now), nil
		}
		d.Execute = func(ctx context.Context) error {
			var newRef string
			if err := h.withBackup(ctx, editor, user, summary, func(ctx context.Context) error {
				ref, err := h.createBouquet(ctx, editor, req.NewName, kind)
				if err != nil {
					return err
				}
				newRef = ref
				return h.applyBouquet(ctx, editor, bouquetDraft{Ref: ref, Kind: kind, Entries: add}, nil, diffBouquet(nil, add))
			}); err != nil {
				return err
			}
			if req.Remember {
				h.rememberIPTV(ctx, sess, newRef, webcontrol.BouquetDisplayName(req.NewName, kind == "radio"), kind, addedEntries)
			}
			h.rememberTwins(ctx, usedTwins)
			h.fetchIPTVLogos(logoJobs)
			return nil
		}
	}
	pl := h.Plans.Create(ctx, user, d)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl, "skipped": skipped,
		"logos": map[string]any{"count": len(logoJobs), "hosts": iptvLogoHosts(logoJobs)}})
}
