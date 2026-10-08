package hubs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Editing the channel list (docs/CHANNELHUB_PLAN.md, C2/C3). The screen
// sends how a bouquet should look when it's done; ChannelHub works out the
// steps and describes them as one plan. Applying: a backup of the whole
// channel list first, then the steps through the BouquetEditor plugin,
// then the bouquet is read back and compared — anything other than what
// was previewed restores the backup. That check also protects against the
// plugin's parameters meaning something slightly different on a box than
// assumed here.

// Error and problem codes.
const (
	CodeChannelsNoEditor  = "channels_no_editor" // the BouquetEditor plugin isn't there
	CodeChannelsUnchanged = "channels_unchanged" // the draft changes nothing
	CodeChannelsRestored  = "channels_restored"  // a step failed or the result differed; the backup was restored
	CodeChannelsBroken    = "channels_restore_failed"
)

func (h *Channels) editor(ctx context.Context) (webcontrol.BouquetEditor, bool) {
	e, ok := h.WC.(webcontrol.BouquetEditor)
	if !ok || !e.BouquetEditorAvailable(ctx) {
		return nil, false
	}
	return e, true
}

func (h *Channels) requireEditor(w http.ResponseWriter, r *http.Request) (webcontrol.BouquetEditor, bool) {
	e, ok := h.editor(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusConflict, CodeChannelsNoEditor, "editing the channel list needs the BouquetEditor plugin on the box")
	}
	return e, ok
}

// DraftEntry is one line of how a bouquet should look: a channel, or a
// heading (a new heading has no reference yet).
type DraftEntry struct {
	Ref    string `json:"ref"`
	Name   string `json:"name"`
	Marker bool   `json:"marker,omitempty"`
}

type bouquetDraft struct {
	Ref     string       `json:"ref"`
	Kind    string       `json:"kind"`
	Entries []DraftEntry `json:"entries"`
}

// ChannelChange is one line of the preview.
type ChannelChange struct {
	Name    string `json:"name"`
	NewName string `json:"newName,omitempty"`
	From    int    `json:"from,omitempty"` // channel number before (headings not counted)
	To      int    `json:"to,omitempty"`   // and after
}

func listFingerprint(list []webcontrol.Service) string {
	sum := sha256.New()
	for _, s := range list {
		fmt.Fprintf(sum, "%s\x00%s\x01", s.ServiceReference, s.ServiceName)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// validate checks the draft; a channel may appear only once.
func (d *bouquetDraft) validate() map[string]httpx.FieldError {
	errs := map[string]httpx.FieldError{}
	if d.Ref == "" {
		errs["ref"] = httpx.FieldError{Code: "required"}
	}
	if d.Kind != "tv" && d.Kind != "radio" {
		errs["kind"] = httpx.FieldError{Code: "invalid"}
	}
	seen := map[string]bool{}
	for i, e := range d.Entries {
		name := strings.TrimSpace(e.Name)
		switch {
		case name == "" || utf8.RuneCountInString(name) > 100:
			errs[fmt.Sprintf("entries.%d.name", i)] = httpx.FieldError{Code: "invalid"}
		case e.Marker:
			if e.Ref != "" && !webcontrol.IsMarker(e.Ref) {
				errs[fmt.Sprintf("entries.%d.ref", i)] = httpx.FieldError{Code: "invalid"}
			}
		case webcontrol.IsMarker(e.Ref) || piconRef.FindString(e.Ref) == "":
			errs[fmt.Sprintf("entries.%d.ref", i)] = httpx.FieldError{Code: "invalid"}
		case seen[channelKey(e.Ref)]:
			errs[fmt.Sprintf("entries.%d.ref", i)] = httpx.FieldError{Code: "duplicate"}
		}
		if !e.Marker {
			seen[channelKey(e.Ref)] = true
		}
		d.Entries[i].Name = name
	}
	return errs
}

// bouquetDiff is what turns the current bouquet into the draft.
type bouquetDiff struct {
	remove     []webcontrol.Service // in the order they are in now
	add        []DraftEntry         // channels
	addMarkers []DraftEntry
	rename     []DraftEntry // with the new name
	moved      map[string]bool
	changes    []plan.Change
}

func (d bouquetDiff) empty() bool {
	return len(d.remove) == 0 && len(d.add) == 0 && len(d.addMarkers) == 0 && len(d.rename) == 0 && len(d.moved) == 0
}

// channelNumbers numbers the channels of a list, headings not counted.
func channelNumbers[T any](list []T, marker func(T) bool) []int {
	out := make([]int, len(list))
	n := 0
	for i, e := range list {
		if !marker(e) {
			n++
			out[i] = n
		}
	}
	return out
}

func diffBouquet(current []webcontrol.Service, draft []DraftEntry) bouquetDiff {
	var d bouquetDiff
	wanted := map[string]bool{}
	for _, e := range draft {
		if e.Ref != "" {
			wanted[e.Ref] = true
		}
	}
	// What goes: anything not in the draft, and a channel's second copy.
	kept := map[string]int{} // ref → index in current
	for i, s := range current {
		_, dup := kept[s.ServiceReference]
		if !wanted[s.ServiceReference] || dup {
			d.remove = append(d.remove, s)
			typ := "channel.remove"
			if webcontrol.IsMarker(s.ServiceReference) {
				typ = "marker.remove"
			}
			d.changes = append(d.changes, plan.Change{Type: typ, Before: ChannelChange{Name: s.ServiceName}})
			continue
		}
		kept[s.ServiceReference] = i
	}
	curNum := channelNumbers(current, func(s webcontrol.Service) bool { return webcontrol.IsMarker(s.ServiceReference) })
	newNum := channelNumbers(draft, func(e DraftEntry) bool { return e.Marker })

	// Kept entries in their new order; the longest run already in order
	// stays, the rest counts as moved.
	var order []int // current indexes, in draft order
	var orderDraft []int
	for j, e := range draft {
		if i, ok := kept[e.Ref]; ok && e.Ref != "" {
			order = append(order, i)
			orderDraft = append(orderDraft, j)
		}
	}
	stay := longestIncreasing(order)
	d.moved = map[string]bool{}
	for k, i := range order {
		if stay[k] {
			continue
		}
		s := current[i]
		d.moved[s.ServiceReference] = true
		if !webcontrol.IsMarker(s.ServiceReference) {
			d.changes = append(d.changes, plan.Change{Type: "channel.move",
				Before: ChannelChange{Name: s.ServiceName, From: curNum[i]}, After: ChannelChange{To: newNum[orderDraft[k]]}})
		} else {
			d.changes = append(d.changes, plan.Change{Type: "marker.move", Before: ChannelChange{Name: s.ServiceName}})
		}
	}
	for j, e := range draft {
		i, ok := kept[e.Ref]
		switch {
		case e.Marker && e.Ref == "":
			d.addMarkers = append(d.addMarkers, e)
			d.changes = append(d.changes, plan.Change{Type: "marker.add", After: ChannelChange{Name: e.Name}})
		case !ok:
			d.add = append(d.add, e)
			d.changes = append(d.changes, plan.Change{Type: "channel.add", After: ChannelChange{Name: e.Name, To: newNum[j]}})
		case current[i].ServiceName != e.Name:
			d.rename = append(d.rename, e)
			d.changes = append(d.changes, plan.Change{Type: "channel.rename", Before: ChannelChange{Name: current[i].ServiceName}, After: ChannelChange{NewName: e.Name}})
		}
	}
	return d
}

// longestIncreasing marks the elements of a longest strictly increasing
// subsequence of xs.
func longestIncreasing(xs []int) []bool {
	n := len(xs)
	length := make([]int, n)
	prev := make([]int, n)
	best := -1
	for i := range xs {
		length[i], prev[i] = 1, -1
		for j := 0; j < i; j++ {
			if xs[j] < xs[i] && length[j]+1 > length[i] {
				length[i], prev[i] = length[j]+1, j
			}
		}
		if best < 0 || length[i] > length[best] {
			best = i
		}
	}
	out := make([]bool, n)
	for i := best; i >= 0; i = prev[i] {
		out[i] = true
	}
	return out
}

func (h *Channels) planBouquet(w http.ResponseWriter, r *http.Request) {
	editor, ok := h.requireEditor(w, r)
	if !ok {
		return
	}
	var d bouquetDraft
	if err := httpx.DecodeJSON(r, &d); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"ref", "kind", "entries": [{"ref", "name", "marker"}]}`)
		return
	}
	if errs := d.validate(); len(errs) > 0 {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "the bouquet draft is invalid; nothing was planned", errs)
		return
	}
	ctx := r.Context()
	current, err := h.WC.Services(ctx, d.Ref)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	diff := diffBouquet(current, d.Entries)
	if diff.empty() {
		httpx.WriteError(w, r, http.StatusConflict, CodeChannelsUnchanged, "the draft is the bouquet as it is")
		return
	}
	user, _ := httpx.User(ctx)
	name := d.Ref
	if list, err := h.allBouquets(ctx); err == nil {
		for _, b := range list {
			if b.Ref == d.Ref {
				name = b.Name
			}
		}
	}
	summary := fmt.Sprintf("Changed bouquet %q: %d changes", name, len(diff.changes))
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "channels.bouquet",
		Changes:     diff.changes,
		Summary:     summary,
		Detail:      map[string]any{"bouquet": name, "changes": diff.changes},
		Fingerprint: listFingerprint(current),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.WC.Services(ctx, d.Ref)
			if err != nil {
				return "", err
			}
			return listFingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			return h.withBackup(ctx, editor, user, summary, func(ctx context.Context) error {
				return h.applyBouquet(ctx, editor, d, current, diff)
			})
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// applyBouquet carries out the steps and checks the result.
func (h *Channels) applyBouquet(ctx context.Context, editor webcontrol.BouquetEditor, d bouquetDraft, current []webcontrol.Service, diff bouquetDiff) error {
	radio := d.Kind == "radio"
	for _, s := range diff.remove {
		if err := editor.RemoveFromBouquet(ctx, d.Ref, s.ServiceReference); err != nil {
			return err
		}
	}
	for _, e := range diff.add {
		if err := editor.AddToBouquet(ctx, d.Ref, e.Ref, "", e.Name); err != nil {
			return err
		}
	}
	before := map[string]bool{}
	for _, s := range current {
		before[s.ServiceReference] = true
	}
	for _, e := range diff.addMarkers {
		if err := editor.AddMarker(ctx, d.Ref, e.Name, ""); err != nil {
			return err
		}
	}
	for _, e := range diff.rename {
		if err := editor.RenameInBouquet(ctx, d.Ref, e.Ref, "", e.Name, radio); err != nil {
			return err
		}
	}
	// The new headings' references, as the box made them: in order of
	// appearance, the headings that weren't there before.
	list, err := h.WC.Services(ctx, d.Ref)
	if err != nil {
		return err
	}
	var newMarkers []string
	for _, s := range list {
		if webcontrol.IsMarker(s.ServiceReference) && !before[s.ServiceReference] {
			newMarkers = append(newMarkers, s.ServiceReference)
		}
	}
	if len(newMarkers) != len(diff.addMarkers) {
		return errors.New("channels: the new headings didn't all appear")
	}
	target := make([]string, 0, len(d.Entries))
	m := 0
	for _, e := range d.Entries {
		if e.Marker && e.Ref == "" {
			target = append(target, newMarkers[m])
			m++
		} else {
			target = append(target, e.Ref)
		}
	}
	// Bring the order in line, one move per entry out of place.
	refs := make([]string, len(list))
	for i, s := range list {
		refs[i] = s.ServiceReference
	}
	for i, want := range target {
		if i < len(refs) && refs[i] == want {
			continue
		}
		at := -1
		for k := i; k < len(refs); k++ {
			if refs[k] == want {
				at = k
				break
			}
		}
		if at < 0 {
			return fmt.Errorf("channels: %q missing after adding", want)
		}
		if err := editor.MoveInBouquet(ctx, d.Ref, want, i, radio); err != nil {
			return err
		}
		refs = append(refs[:at], refs[at+1:]...)
		refs = append(refs[:i], append([]string{want}, refs[i:]...)...)
	}
	// Verify, don't trust.
	after, err := h.WC.Services(ctx, d.Ref)
	if err != nil {
		return err
	}
	if len(after) != len(target) {
		return fmt.Errorf("channels: the bouquet has %d entries, %d expected", len(after), len(target))
	}
	for i, s := range after {
		if s.ServiceReference != target[i] {
			return fmt.Errorf("channels: entry %d is %q, %q expected", i, s.ServiceReference, target[i])
		}
	}
	for _, e := range diff.rename {
		for _, s := range after {
			if s.ServiceReference == e.Ref && s.ServiceName != e.Name {
				return fmt.Errorf("channels: %q wasn't renamed", e.Ref)
			}
		}
	}
	return nil
}

// withBackup saves the whole channel list, runs change, and restores the
// backup when change fails — so the list is never left half-changed.
func (h *Channels) withBackup(ctx context.Context, editor webcontrol.BouquetEditor, user store.User, summary string, change func(context.Context) error) error {
	// The time for people, a random part so two changes in one second
	// can't overwrite each other's backup.
	name := "dreamhub-" + h.now().UTC().Format("20060102-150405") + "-" + randomHex(3)
	if err := editor.EditorBackup(ctx, name); err != nil {
		if errors.Is(err, webcontrol.ErrWritesDisabled) {
			return err
		}
		return &plan.Failure{Status: http.StatusBadGateway, Code: CodeChannelsBroken, Message: "the backup before the change failed; nothing was changed", Err: err}
	}
	_ = h.Store.AddChannelBackup(ctx, name, summary, user.ID)
	if err := change(ctx); err != nil {
		if rerr := editor.EditorRestore(ctx, name); rerr != nil {
			return &plan.Failure{Status: http.StatusBadGateway, Code: CodeChannelsBroken,
				Message: "the change failed and restoring the backup failed too", Details: map[string]any{"backup": name}, Err: err}
		}
		return &plan.Failure{Status: http.StatusConflict, Code: CodeChannelsRestored,
			Message: "the change didn't go through as previewed; the channel list was restored from the backup", Details: map[string]any{"backup": name}, Err: err}
	}
	return nil
}

// --- bouquets themselves --------------------------------------------------------

type bouquetAction struct {
	Action string `json:"action"` // "create", "rename", "delete"
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Name   string `json:"name"`
}

func (h *Channels) planBouquets(w http.ResponseWriter, r *http.Request) {
	editor, ok := h.requireEditor(w, r)
	if !ok {
		return
	}
	var a bouquetAction
	if err := httpx.DecodeJSON(r, &a); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"action": "create"|"rename"|"delete", "kind", "ref", "name"}`)
		return
	}
	a.Name = strings.TrimSpace(a.Name)
	ctx := r.Context()
	list, err := h.allBouquets(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	var existing *BouquetSummary
	for i := range list {
		if list[i].Ref == a.Ref {
			existing = &list[i]
		}
	}
	errs := map[string]httpx.FieldError{}
	if a.Kind != "tv" && a.Kind != "radio" {
		errs["kind"] = httpx.FieldError{Code: "invalid"}
	}
	if a.Action != "delete" && (a.Name == "" || utf8.RuneCountInString(a.Name) > 60) {
		errs["name"] = httpx.FieldError{Code: "invalid"}
	}
	if a.Action != "create" && existing == nil {
		errs["ref"] = httpx.FieldError{Code: "not_found"}
	}
	if a.Action != "create" && a.Action != "rename" && a.Action != "delete" {
		errs["action"] = httpx.FieldError{Code: "invalid"}
	}
	if len(errs) > 0 {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was planned", errs)
		return
	}
	radio := a.Kind == "radio"
	var change plan.Change
	var summary string
	var do func(context.Context) error
	var check func([]BouquetSummary) bool
	switch a.Action {
	case "create":
		change = plan.Change{Type: "bouquet.create", After: ChannelChange{Name: a.Name}}
		summary = fmt.Sprintf("Created bouquet %q", a.Name)
		do = func(ctx context.Context) error { return editor.AddBouquet(ctx, a.Name, radio) }
		check = func(after []BouquetSummary) bool { return len(after) == len(list)+1 && hasBouquetNamed(after, a.Name) }
	case "rename":
		change = plan.Change{Type: "bouquet.rename", Before: ChannelChange{Name: existing.Name}, After: ChannelChange{NewName: a.Name}}
		summary = fmt.Sprintf("Renamed bouquet %q to %q", existing.Name, a.Name)
		do = func(ctx context.Context) error { return editor.RenameBouquet(ctx, a.Ref, a.Name, radio) }
		check = func(after []BouquetSummary) bool {
			for _, b := range after {
				if b.Ref == a.Ref {
					return b.Name == a.Name
				}
			}
			return false
		}
	case "delete":
		change = plan.Change{Type: "bouquet.delete", Before: ChannelChange{Name: existing.Name}}
		summary = fmt.Sprintf("Deleted bouquet %q", existing.Name)
		do = func(ctx context.Context) error { return editor.RemoveBouquet(ctx, a.Ref, radio) }
		check = func(after []BouquetSummary) bool {
			for _, b := range after {
				if b.Ref == a.Ref {
					return false
				}
			}
			return len(after) == len(list)-1
		}
	}
	user, _ := httpx.User(ctx)
	fp := func(l []BouquetSummary) string {
		var s []webcontrol.Service
		for _, b := range l {
			s = append(s, webcontrol.Service{ServiceReference: b.Ref, ServiceName: b.Name})
		}
		return listFingerprint(s)
	}
	pl := h.Plans.Create(user, plan.Draft{
		Action: "channels." + change.Type, Changes: []plan.Change{change}, Summary: summary, Detail: a,
		Fingerprint: fp(list),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.allBouquets(ctx)
			if err != nil {
				return "", err
			}
			return fp(now), nil
		},
		Execute: func(ctx context.Context) error {
			return h.withBackup(ctx, editor, user, summary, func(ctx context.Context) error {
				if err := do(ctx); err != nil {
					return err
				}
				after, err := h.allBouquets(ctx)
				if err != nil {
					return err
				}
				if !check(after) {
					return errors.New("channels: the bouquet list isn't as previewed")
				}
				return nil
			})
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

func hasBouquetNamed(list []BouquetSummary, name string) bool {
	for _, b := range list {
		if b.Name == name {
			return true
		}
	}
	return false
}

// --- backups (C3) -----------------------------------------------------------------

func (h *Channels) backups(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ChannelBackups(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not list the backups")
		return
	}
	type view struct {
		Filename  string    `json:"filename"`
		CreatedAt time.Time `json:"createdAt"`
		Summary   string    `json:"summary"`
	}
	out := make([]view, 0, len(list))
	for _, b := range list {
		out = append(out, view{b.Filename, b.CreatedAt, b.Summary})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"backups": out})
}

// planRestore previews bringing a backup back — itself a change, so the
// list as it is now is backed up first.
func (h *Channels) planRestore(w http.ResponseWriter, r *http.Request) {
	editor, ok := h.requireEditor(w, r)
	if !ok {
		return
	}
	var body struct {
		Filename string `json:"filename"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"filename"}`)
		return
	}
	ctx := r.Context()
	list, err := h.Store.ChannelBackups(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not list the backups")
		return
	}
	var b *store.ChannelBackup
	for i := range list {
		if list[i].Filename == body.Filename {
			b = &list[i]
		}
	}
	if b == nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such backup")
		return
	}
	user, _ := httpx.User(ctx)
	summary := fmt.Sprintf("Restored the channel list from %s", b.Filename)
	pl := h.Plans.Create(user, plan.Draft{
		Action:  "channels.restore",
		Changes: []plan.Change{{Type: "channels.restore", After: map[string]any{"filename": b.Filename, "createdAt": b.CreatedAt, "summary": b.Summary}}},
		Summary: summary, Detail: map[string]any{"filename": b.Filename},
		Execute: func(ctx context.Context) error {
			return h.withBackup(ctx, editor, user, summary, func(ctx context.Context) error {
				return editor.EditorRestore(ctx, b.Filename)
			})
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}
