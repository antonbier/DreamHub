package hubs

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Feeds on and off, and a new one, as a plan (docs/ADMINHUB_PLAN.md,
// "Feeds"). A feed is a place packages come from, with root rights on the
// box and without signatures (ARCHITECTURE.md, "Package feeds are
// unsigned") — so adding one always shows a warning, and every change
// refreshes apt's index so the package list matches what the feeds offer.

// CodeFeedUnsigned warns on adding a feed (params: url).
const CodeFeedUnsigned = "feed.unsigned"

func feedsFingerprint(feeds []apt.Feed) string {
	var b strings.Builder
	for _, f := range feeds {
		b.WriteString(f.Name)
		if f.Enabled {
			b.WriteString("+")
		}
		b.WriteString("|")
	}
	return b.String()
}

// planFeed answers POST /api/v1/admin/feeds/plan with
// {"action": "enable"|"disable"|"add", "name": "...", "url": "..."}.
func (h *Admin) planFeed(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
		Name   string `json:"name"`
		URL    string `json:"url"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || !apt.ValidFeedName(body.Name) ||
		(body.Action != "enable" && body.Action != "disable" && body.Action != "add") ||
		(body.Action == "add" && !apt.ValidFeedURL(body.URL)) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"action": "enable|disable|add", "name": "a-feed-name", "url": "https://… (add only)"}`)
		return
	}
	if h.Feeds == nil || h.FeedManager == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "feeds can't be changed on this system")
		return
	}
	ctx := r.Context()
	before, err := h.Feeds.ListFeeds(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "apt_failed", "could not read the feed list")
		return
	}
	var current *apt.Feed
	for i := range before {
		if before[i].Name == body.Name {
			current = &before[i]
		}
	}
	var warnings []status.Problem
	var summary string
	switch body.Action {
	case "add":
		if current != nil {
			httpx.WriteError(w, r, http.StatusConflict, "feed_exists", "a feed with this name already exists")
			return
		}
		summary = "Added feed " + body.Name + " (" + body.URL + ")"
		warnings = append(warnings, status.Problem{Code: CodeFeedUnsigned, Params: map[string]any{"url": body.URL}})
	default:
		on := body.Action == "enable"
		if current == nil {
			httpx.WriteError(w, r, http.StatusNotFound, "feed_not_found", "no such feed")
			return
		}
		if current.Enabled == on {
			httpx.WriteError(w, r, http.StatusConflict, "nothing_to_do", "the feed is already so")
			return
		}
		summary = map[bool]string{true: "Switched on feed ", false: "Switched off feed "}[on] + body.Name
	}

	user, _ := httpx.User(ctx)
	action, name, url := body.Action, body.Name, body.URL
	pl := h.Plans.Create(ctx, user, plan.Draft{
		Action:      "admin.feed." + action,
		Changes:     []plan.Change{{Type: "feed." + action, After: map[string]string{"name": name, "url": url}}},
		Warnings:    warnings,
		Summary:     summary,
		Fingerprint: feedsFingerprint(before),
		Current: func(ctx context.Context) (string, error) {
			now, err := h.Feeds.ListFeeds(ctx)
			if err != nil {
				return "", err
			}
			return feedsFingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			var undo func()
			var err error
			switch action {
			case "add":
				err = h.FeedManager.AddFeed(ctx, name, url)
				undo = func() { _ = h.FeedManager.RemoveFeed(context.WithoutCancel(ctx), name) }
			default:
				on := action == "enable"
				err = h.FeedManager.SetFeedEnabled(ctx, name, on)
				undo = func() { _ = h.FeedManager.SetFeedEnabled(context.WithoutCancel(ctx), name, !on) }
			}
			if err != nil {
				if errors.Is(err, apt.ErrFeedExists) || errors.Is(err, apt.ErrFeedMissing) {
					return &plan.Failure{Status: http.StatusConflict, Code: "plan_stale", Message: "the feed list changed", Err: err}
				}
				return &plan.Failure{Status: http.StatusBadGateway, Code: "feed_failed", Message: "could not change the feed", Err: err}
			}
			// A feed whose index can't be fetched would leave every later
			// apt run complaining — put the file back the way it was.
			if err := h.FeedManager.RefreshIndex(ctx); err != nil {
				undo()
				return &plan.Failure{Status: http.StatusBadGateway, Code: "feed_unreachable", Message: "apt couldn't read the feed; nothing was changed", Err: err}
			}
			_ = h.Store.AppendChange(context.WithoutCancel(ctx), store.ChangeEntry{UserID: user.ID, Action: "admin.feed." + action, Summary: summary})
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}
