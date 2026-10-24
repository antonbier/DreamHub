package hubs

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/iptv"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// What the Netcast plugin keeps of the sources it has loaded, in
// /data/Netcast/server (read on the One 2026-10-24; ARCHITECTURE.md): an M3U
// per playlist source, Samsung TV Plus's list as gzipped JSON, and for Pluto
// only the channel names and ids its guide loader knows. The lists are the
// ones the TV shows; DreamHub fetches nothing for them and writes the
// entries the way the plugin does (iptv.NativeRef).

// errNetcastUnsupported: a source whose channels DreamHub cannot list (Vavoo
// plays through a local proxy and cannot go into a bouquet; the others serve
// films and series, not channels).
var errNetcastUnsupported = errors.New("hubs: this Netcast source has no channel list DreamHub can use")

const (
	// Vavoo (docs/NETCAST_PLAN.md, "Vavoo"): the plugin keeps the channel list
	// of its Vavoo addon in /data/Netcast/server/vavoo.livetv.json and plays
	// every channel through a proxy of its own (127.0.0.1:4323), which the
	// addon starts only when its Vavoo screen is opened at the TV
	// (internal/vavooproxy starts it when it is not there; checked on the One
	// 2026-10-24). DreamHub reads that list and hands the proxy address to the
	// box's player; it never talks to Vavoo itself.
	netcastVavooID    = "vavooapi"
	netcastVavooFile  = "vavoo.livetv.json"
	netcastVavooProxy = "http://127.0.0.1:4323/vavoo?url="
	netcastSamsungID  = "samsungtvplusapi"
	netcastPlutoID    = "plutotvapi"
	netcastPlutoBase  = "http://cfd-v4-service-channel-stitcher-use1-1.prd.pluto.tv/v2/stitch/hls/channel/"
)

// netcastCachedList reads the cached list of source id.
func netcastCachedList(ctx context.Context, rd webcontrol.BoxFileReader, id string) (*iptv.Playlist, error) {
	switch id {
	case netcastSamsungID:
		raw, err := rd.ReadBoxFileMax(ctx, path.Join(netcastCacheDir, "netcastserver_"+id), iptvMaxUploadSize)
		if err != nil {
			return nil, err
		}
		return parseSamsungList(raw)
	case netcastPlutoID:
		raw, err := rd.ReadBoxFileMax(ctx, path.Join(netcastCacheDir, "epgChannelList_netcastserver_"+id+".json"), iptvMaxUploadSize)
		if err != nil {
			return nil, err
		}
		return parsePlutoList(raw)
	}
	if strings.HasSuffix(id, "api") || strings.HasPrefix(id, "vavoo") {
		return nil, errNetcastUnsupported
	}
	raw, err := rd.ReadBoxFileMax(ctx, path.Join(netcastCacheDir, "playlist_netcastserver_"+id), iptvMaxUploadSize)
	if err != nil {
		return nil, err
	}
	return parseNetcastPlaylist(id, raw)
}

// parseNetcastPlaylist reads an M3U of playlist source id, as cached by the
// plugin or fetched by DreamHub from the source's own address, and marks its
// entries the way the plugin does.
func parseNetcastPlaylist(id string, raw []byte) (*iptv.Playlist, error) {
	pl, err := iptv.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	for i := range pl.Entries {
		// The plugin writes an https address without its scheme after
		// "netcastserver_<id>://"; for another scheme nothing is known, so
		// that entry stays a plain stream.
		if rest, ok := strings.CutPrefix(pl.Entries[i].URL, "https://"); ok {
			pl.Entries[i].Netcast, pl.Entries[i].NetcastURL = id, rest
		}
	}
	return pl, nil
}

// vavooRank orders a channel's streams: the plain ones first, the ones the
// box's player is least likely to take (HEVC/H.265, RAW, a backup, a local
// or satellite feed) last. A guess from the names — which stream plays is
// found out on the TV, and the page lets one pick.
func vavooRank(name string) int {
	n := strings.ToUpper(name)
	rank := 0
	for _, w := range []string{"RAW", "HEVC", "H265", "UHD", "LOCAL"} {
		if strings.Contains(n, w) {
			rank += 20
		}
	}
	if strings.Contains(n, "BACKUP") {
		rank += 10
	}
	if strings.Contains(n, "[") {
		rank += 5
	}
	if strings.Contains(n, "+24") {
		rank += 3
	}
	return rank
}

// netcastVavooList reads Vavoo's cached channel list. Not part of
// netcastCachedList: only NetcastHub plays it, and it never goes into a
// bouquet (the entry is a proxy address that means something on this box only).
func netcastVavooList(ctx context.Context, rd webcontrol.BoxFileReader) (*iptv.Playlist, error) {
	raw, err := rd.ReadBoxFileMax(ctx, path.Join(netcastCacheDir, netcastVavooFile), iptvMaxUploadSize)
	if err != nil {
		return nil, err
	}
	return parseVavooList(raw)
}

// parseVavooList reads {"<country>": {"<channel>": {"url", "logo", "tvgid",
// "streams": [{"url", "type": "HLS-Vavoo", "name"}]}}}. A channel's group is
// its country; its address is the plugin's proxy with the first Vavoo
// stream.
func parseVavooList(raw []byte) (*iptv.Playlist, error) {
	var doc map[string]map[string]struct {
		Logo    string `json:"logo"`
		TvgID   string `json:"tvgid"`
		Streams []struct {
			URL  string `json:"url"`
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc) == 0 {
		return nil, &iptv.FetchError{Kind: iptv.KindNotPlaylist}
	}
	pl := &iptv.Playlist{}
	countries := make([]string, 0, len(doc))
	for c := range doc {
		countries = append(countries, c)
	}
	sort.Strings(countries)
	for _, country := range countries {
		names := make([]string, 0, len(doc[country]))
		for n := range doc[country] {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
		for _, name := range names {
			ch := doc[country][name]
			var alts []iptv.Alt
			seen := map[string]bool{}
			for _, st := range ch.Streams {
				if st.Type != "HLS-Vavoo" || !strings.HasPrefix(st.URL, "https://") || seen[st.URL] {
					continue
				}
				seen[st.URL] = true
				// The plugin writes it percent-encoded into the proxy's
				// query (seen in `getcurrent` on the One).
				alts = append(alts, iptv.Alt{Name: strings.TrimSpace(st.Name), URL: netcastVavooProxy + url.QueryEscape(st.URL)})
			}
			if len(alts) == 0 || strings.TrimSpace(name) == "" {
				continue
			}
			sort.SliceStable(alts, func(i, j int) bool { return vavooRank(alts[i].Name) < vavooRank(alts[j].Name) })
			if len(pl.Entries) >= iptv.MaxEntries {
				pl.Truncated = true
				break
			}
			e := iptv.Entry{Name: strings.TrimSpace(name), URL: alts[0].URL, Alts: alts, Group: country, TvgID: ch.TvgID}
			if strings.HasPrefix(ch.Logo, "http") {
				e.Logo = ch.Logo
			}
			pl.Entries = append(pl.Entries, e)
		}
	}
	if len(pl.Entries) == 0 {
		return nil, &iptv.FetchError{Kind: iptv.KindNotPlaylist}
	}
	return pl, nil
}

// parseSamsungList reads Samsung TV Plus's cache: {"slug": "stvp-{id}",
// "regions": {"de": {"name", "channels": {"<id>": {"name", "chno", "group",
// "logo", ...}}}}}, gzipped. A channel is "jmp2.uk/stvp-<id>" (the plugin's
// own address, samsungtvplusapi_parser.py); it leaves out the three the
// plugin leaves out.
func parseSamsungList(raw []byte) (*iptv.Playlist, error) {
	if zr, err := gzip.NewReader(bytes.NewReader(raw)); err == nil {
		raw, err = io.ReadAll(io.LimitReader(zr, 32<<20))
		if err != nil {
			return nil, &iptv.FetchError{Kind: iptv.KindNotPlaylist}
		}
	}
	var doc struct {
		Regions map[string]struct {
			Name     string `json:"name"`
			Channels map[string]struct {
				Name  string `json:"name"`
				Chno  int    `json:"chno"`
				Group string `json:"group"`
				Logo  string `json:"logo"`
			} `json:"channels"`
		} `json:"regions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Regions) == 0 {
		return nil, &iptv.FetchError{Kind: iptv.KindNotPlaylist}
	}
	type row struct {
		region string
		chno   int
		e      iptv.Entry
	}
	var rows []row
	for code, reg := range doc.Regions {
		regName := firstOf(reg.Name, strings.ToUpper(code))
		for cid, c := range reg.Channels {
			name := strings.TrimSpace(c.Name)
			if name == "" || strings.Contains(name, "Sony One") || strings.Contains(name, "DAZN") || strings.Contains(name, "Beverly Hills") {
				continue
			}
			group := regName
			if g := strings.TrimSpace(c.Group); g != "" {
				group += " · " + g
			}
			rows = append(rows, row{regName, c.Chno, iptv.Entry{
				Name: name, Group: group, TvgID: cid, Logo: c.Logo,
				URL: "https://jmp2.uk/stvp-" + cid, Netcast: netcastSamsungID, NetcastURL: "jmp2.uk/stvp-" + cid,
			}})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.region == "Germany") != (b.region == "Germany") {
			return a.region == "Germany"
		}
		if a.region != b.region {
			return a.region < b.region
		}
		if a.chno != b.chno {
			return a.chno < b.chno
		}
		return a.e.Name < b.e.Name
	})
	pl := &iptv.Playlist{}
	for _, r := range rows {
		pl.Entries = append(pl.Entries, r.e)
	}
	return pl, nil
}

// parsePlutoList reads Pluto's guide channel list ({"entrys": [{"id", "name",
// "active"}]}): all the plugin keeps of Pluto's live channels. The address is
// the channel's stitcher address with its id; the plugin adds the session
// when it plays one.
func parsePlutoList(raw []byte) (*iptv.Playlist, error) {
	var doc struct {
		Entrys []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"entrys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Entrys) == 0 {
		return nil, &iptv.FetchError{Kind: iptv.KindNotPlaylist}
	}
	pl := &iptv.Playlist{}
	for _, c := range doc.Entrys {
		name := strings.TrimSpace(c.Name)
		if name == "" || !netcastIDPattern.MatchString(c.ID) {
			continue
		}
		pl.Entries = append(pl.Entries, iptv.Entry{
			Name: name, Group: "Pluto TV", TvgID: c.ID,
			URL: netcastPlutoBase + c.ID + "/", Netcast: netcastPlutoID, NetcastURL: netcastPlutoBase + c.ID + "/",
		})
	}
	sort.SliceStable(pl.Entries, func(i, j int) bool {
		return strings.ToLower(pl.Entries[i].Name) < strings.ToLower(pl.Entries[j].Name)
	})
	return pl, nil
}
