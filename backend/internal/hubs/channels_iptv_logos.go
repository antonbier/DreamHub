package hubs

import (
	"context"
	"log/slog"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/iptv"
)

// Logos for internet channels (docs/NETCAST_PLAN.md, N1). A list gives a
// channel's logo as an address on its own server (tvg-logo). When the person
// ticks "logos" in the preview, the box fetches them after the change is
// confirmed — one at a time per slot, small ones only, never over a logo the
// person chose by hand — and stores them like any hand-chosen logo, under the
// key of the channel's reference as the box will list it ("…:<stream>:<name>").
// A logo that cannot be fetched only costs that logo.

const (
	iptvLogoMax      = 2 << 20
	iptvLogoMany     = 400 // logos fetched for one change at most
	iptvLogoParallel = 4
	iptvLogoEach     = 10 * time.Second
	iptvLogoAll      = 5 * time.Minute
)

type iptvLogoJob struct {
	ref string // the reference the channel has in the bouquet, with its name
	url string
}

// iptvLogoJobFor makes the job for an entry with a logo address, or false.
func iptvLogoJobFor(e iptv.Entry, listedRef string) (iptvLogoJob, bool) {
	if e.Logo == "" || logoDirectory() == "" {
		return iptvLogoJob{}, false
	}
	u, err := url.Parse(e.Logo)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return iptvLogoJob{}, false
	}
	return iptvLogoJob{ref: listedRef, url: e.Logo}, true
}

// iptvLogoHosts names the servers the logos come from, for the preview.
func iptvLogoHosts(jobs []iptvLogoJob) []string {
	seen := map[string]bool{}
	var out []string
	for _, j := range jobs {
		if u, err := url.Parse(j.url); err == nil && !seen[u.Hostname()] {
			seen[u.Hostname()] = true
			out = append(out, u.Hostname())
		}
	}
	sort.Strings(out)
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// fetchIPTVLogos fetches the logos in the background.
func (h *Channels) fetchIPTVLogos(jobs []iptvLogoJob) {
	if len(jobs) == 0 || logoDirectory() == "" {
		return
	}
	if len(jobs) > iptvLogoMany {
		jobs = jobs[:iptvLogoMany]
	}
	f := h.fetcher()
	h.logoWG.Add(1)
	go func() {
		defer h.logoWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), iptvLogoAll)
		defer cancel()
		sem := make(chan struct{}, iptvLogoParallel)
		var wg sync.WaitGroup
		for _, j := range jobs {
			if ctx.Err() != nil {
				break
			}
			sem <- struct{}{}
			wg.Add(1)
			go func(j iptvLogoJob) {
				defer func() { <-sem; wg.Done() }()
				if hasLogoOverride(j.ref) {
					return // chosen by hand before: not replaced
				}
				c, cancel := context.WithTimeout(ctx, iptvLogoEach)
				defer cancel()
				data, err := f.FetchLimited(c, j.url, iptvLogoMax)
				if err != nil {
					return
				}
				png, err := normalizeLogo(data)
				if err != nil {
					return
				}
				if err := storeLogo(j.ref, png); err != nil {
					slog.Warn("could not store a channel logo", "err", err)
				}
			}(j)
		}
		wg.Wait()
	}()
}

// moveLogo gives a channel whose address changed the logo it had: a logo is
// kept under a key that holds a hash of the address.
func moveLogo(oldRef, newRef string) {
	if oldRef == newRef || logoDirectory() == "" {
		return
	}
	if b, ok := readLogoOverride(oldRef); ok && !hasLogoOverride(newRef) {
		_ = storeLogo(newRef, b)
	}
}
