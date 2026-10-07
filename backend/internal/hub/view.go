package hub

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/status"
)

// Views are the hub contract's data source for custom dashboard tiles
// (CONCEPT.md, "Custom tiles = saved views with criteria": source + filters
// + sort + limit + display). A hub offers one or more sources and returns
// its candidate items; the filters, sort and limit are applied here, the
// same way for every source, over the few fields every item has. A source
// only lists the filters and sorts that make sense for its items, so the
// tile builder offers nothing that would silently do nothing.
//
// Views only read. A tile never acts on its own (CONCEPT.md).

// Filter names a source can support.
const (
	FilterText          = "text"          // title or subtitle contains, case-insensitive
	FilterOlderThanDays = "olderThanDays" // Time at least N days ago
	FilterNewerThanDays = "newerThanDays" // Time at most N days ago
	FilterMinSizeGB     = "minSizeGB"     // Size at least N GB
	FilterWithinDays    = "withinDays"    // Time before now + N days
	FilterFromTime      = "fromTime"      // starts at or after HH:MM local time, any day
	FilterMinDuration   = "minDuration"   // End - Time at least N minutes
	FilterWarnOnly      = "warnOnly"      // only items flagged Warn (e.g. timer conflicts)
	FilterBouquet       = "bouquet"       // which bouquet (GuideHub); used by the hub itself
)

// Sort orders a source can support.
const (
	SortNewest  = "newest"  // Time descending
	SortOldest  = "oldest"  // Time ascending
	SortSoonest = "soonest" // Time ascending, for things still to come
	SortLargest = "largest" // Size descending
)

// ViewSource describes one source a hub offers.
type ViewSource struct {
	ID      string   `json:"id"`    // e.g. "media.recordings"
	Label   string   `json:"label"` // i18n key
	HubID   string   `json:"hubId"`
	Filters []string `json:"filters"`
	Sorts   []string `json:"sorts"` // the first is the default
}

// ViewQuery is a custom tile's criteria. Zero values mean "no filter".
type ViewQuery struct {
	Text          string  `json:"text,omitempty"`
	OlderThanDays int     `json:"olderThanDays,omitempty"`
	NewerThanDays int     `json:"newerThanDays,omitempty"`
	MinSizeGB     float64 `json:"minSizeGB,omitempty"`
	WithinDays    int     `json:"withinDays,omitempty"`
	FromTime      string  `json:"fromTime,omitempty"` // "HH:MM"
	MinDuration   int     `json:"minDuration,omitempty"`
	WarnOnly      bool    `json:"warnOnly,omitempty"`
	Bouquet       string  `json:"bouquet,omitempty"`
	Sort          string  `json:"sort,omitempty"`
	Limit         int     `json:"limit,omitempty"`
}

// MaxViewLimit caps how many items a view returns.
const MaxViewLimit = 20

// ViewItem is one row a view shows.
type ViewItem struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Time     int64  `json:"time,omitempty"` // Unix seconds, UTC
	End      int64  `json:"end,omitempty"`
	Size     int64  `json:"size,omitempty"` // bytes
	Warn     bool   `json:"warn,omitempty"`
	Path     string `json:"path,omitempty"` // where the item opens in the UI
}

// ViewResult is a view's items after filter, sort and limit, plus the
// number of matches before the limit and their total size (for the
// "12 matches, 48 GB" line).
type ViewResult struct {
	Items     []ViewItem `json:"items"`
	Total     int        `json:"total"`
	TotalSize int64      `json:"totalSize,omitempty"`
}

// ViewProvider is implemented by a hub that offers sources for custom
// tiles. View returns every candidate item of sourceID; q is passed along
// for filters only the hub can apply (FilterBouquet, and how far ahead to
// fetch for FilterWithinDays).
type ViewProvider interface {
	ViewSources() []ViewSource
	View(ctx context.Context, sourceID string, q ViewQuery, now time.Time) ([]ViewItem, error)
}

// AddViewProvider registers p for hubID. Call after Add for that hub.
func (r *Registry) AddViewProvider(hubID string, p ViewProvider) {
	r.views[hubID] = p
}

// ErrUnknownSource is returned by View for a source no hub offers.
var ErrUnknownSource = errors.New("hub: unknown view source")

// ErrSourceUnavailable is returned by View when the source's hub is
// disabled, hidden from the user's role or currently unavailable.
var ErrSourceUnavailable = errors.New("hub: view source unavailable")

// KnownSource reports whether any hub offers sourceID at all, regardless
// of its current state — what the dashboard validates a saved tile
// against, so a tile survives its hub being briefly unavailable.
func (r *Registry) KnownSource(sourceID string) bool {
	_, ok := r.sourceHub(sourceID)
	return ok
}

func (r *Registry) sourceHub(sourceID string) (string, bool) {
	for hubID, p := range r.views {
		for _, s := range p.ViewSources() {
			if s.ID == sourceID {
				return hubID, true
			}
		}
	}
	return "", false
}

// ViewSources lists the sources the user can use right now, in navigation
// order — same visibility rule as Search.
func (r *Registry) ViewSources(v settings.Values, reports map[string]connector.Report, perms perm.Set) []ViewSource {
	statuses := r.Statuses(v, reports)
	order := map[string]int{}
	for _, d := range r.hubs {
		order[d.ID] = d.Order
	}
	sort.SliceStable(statuses, func(i, j int) bool { return order[statuses[i].ID] < order[statuses[j].ID] })
	out := []ViewSource{}
	for _, s := range statuses {
		if !s.Enabled || !Visible(s.Perm, perms) || s.Level == status.Unavailable {
			continue
		}
		p, ok := r.views[s.ID]
		if !ok {
			continue
		}
		for _, src := range p.ViewSources() {
			src.HubID = s.ID
			out = append(out, src)
		}
	}
	return out
}

// View evaluates q against sourceID for a user with perms.
func (r *Registry) View(ctx context.Context, v settings.Values, reports map[string]connector.Report, perms perm.Set, sourceID string, q ViewQuery, now time.Time, loc *time.Location) (ViewResult, error) {
	hubID, ok := r.sourceHub(sourceID)
	if !ok {
		return ViewResult{}, ErrUnknownSource
	}
	var src ViewSource
	usable := false
	for _, s := range r.ViewSources(v, reports, perms) {
		if s.ID == sourceID {
			src, usable = s, true
		}
	}
	if !usable {
		return ViewResult{}, ErrSourceUnavailable
	}
	items, err := r.views[hubID].View(ctx, sourceID, q, now)
	if err != nil {
		return ViewResult{}, err
	}
	return applyView(items, src, q, now, loc), nil
}

// applyView filters, sorts and limits items. Filters the source doesn't
// list are ignored, so a stale value in a saved tile can't hide items.
func applyView(items []ViewItem, src ViewSource, q ViewQuery, now time.Time, loc *time.Location) ViewResult {
	has := map[string]bool{}
	for _, f := range src.Filters {
		has[f] = true
	}
	if loc == nil {
		loc = time.UTC
	}
	fromMin, fromOK := parseClock(q.FromTime)
	text := strings.ToLower(strings.TrimSpace(q.Text))
	nowUnix := now.Unix()

	out := []ViewItem{}
	var size int64
	for _, it := range items {
		switch {
		case has[FilterText] && text != "" &&
			!strings.Contains(strings.ToLower(it.Title), text) && !strings.Contains(strings.ToLower(it.Subtitle), text):
			continue
		case has[FilterOlderThanDays] && q.OlderThanDays > 0 && it.Time > nowUnix-int64(q.OlderThanDays)*86400:
			continue
		case has[FilterNewerThanDays] && q.NewerThanDays > 0 && it.Time < nowUnix-int64(q.NewerThanDays)*86400:
			continue
		case has[FilterMinSizeGB] && q.MinSizeGB > 0 && float64(it.Size) < q.MinSizeGB*1e9:
			continue
		case has[FilterWithinDays] && q.WithinDays > 0 && it.Time >= nowUnix+int64(q.WithinDays)*86400:
			continue
		case has[FilterMinDuration] && q.MinDuration > 0 && (it.End-it.Time) < int64(q.MinDuration)*60:
			continue
		case has[FilterWarnOnly] && q.WarnOnly && !it.Warn:
			continue
		}
		if has[FilterFromTime] && fromOK {
			t := time.Unix(it.Time, 0).In(loc)
			if t.Hour()*60+t.Minute() < fromMin {
				continue
			}
		}
		out = append(out, it)
		size += it.Size
	}

	sortBy := q.Sort
	if !contains(src.Sorts, sortBy) && len(src.Sorts) > 0 {
		sortBy = src.Sorts[0]
	}
	switch sortBy {
	case SortNewest:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	case SortOldest, SortSoonest:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	case SortLargest:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	}

	res := ViewResult{Total: len(out), TotalSize: size}
	limit := q.Limit
	if limit <= 0 || limit > MaxViewLimit {
		limit = 5
	}
	if len(out) > limit {
		out = out[:limit]
	}
	res.Items = out
	return res
}

// parseClock parses "HH:MM" into minutes after midnight.
func parseClock(s string) (int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	return hh*60 + mm, true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
