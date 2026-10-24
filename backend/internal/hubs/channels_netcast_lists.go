package hubs

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	netcastSamsungID = "samsungtvplusapi"
	netcastPlutoID   = "plutotvapi"
	netcastPlutoBase = "http://cfd-v4-service-channel-stitcher-use1-1.prd.pluto.tv/v2/stitch/hls/channel/"
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
