package mock

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The mock box's files (webcontrol.BoxFileReader): Netcast's source files in
// the layout of a Dreambox One with the Gemini plugins (ARCHITECTURE.md,
// "Netcast sources on the One"), with made-up addresses.
var _ webcontrol.BoxFileReader = (*Client)(nil)

func mockBoxFiles() map[string][]byte {
	return map[string][]byte{
		"/etc/enigma2/netcastserver_gemini.json": []byte(`{"serverlist":[
			{"id":"kodi","name":"Kodi IPTV","mainUrl":"https://iptv.example/kodi/playlist.m3u","mediatype":"playlist_live","desc":"Live"},
			{"id":"lgat","name":"LG Channels AT","mainUrl":"https://iptv.example/lg/at.m3u8","mediatype":"playlist_live"}]}`),
		"/etc/enigma2/netcastserver_plutotv.json": []byte(`{"serverlist":[
			{"id":"pluto","name":"Pluto TV","mediatype":"api","epgUpdate":"3600","Username":"me","Password":"hunter2"}]}`),
		"/etc/enigma2/netcastepg_gemini.json": []byte(`{"epglist":[
			{"id":"gemini","name":"XMLTV DE","epgUrl":"https://epg.example/de.xml.gz","EPGrefreshHour":"96"}]}`),
	}
}

// ReadBoxFile returns one of the mock box's files.
func (c *Client) ReadBoxFile(ctx context.Context, p string) ([]byte, error) {
	if c.NoBoxFiles {
		return nil, fmt.Errorf("mock: %w", webcontrol.ErrNotFound)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if b, ok := c.boxFiles()[p]; ok {
		return append([]byte(nil), b...), nil
	}
	return nil, fmt.Errorf("mock: %s: %w", p, webcontrol.ErrNotFound)
}

// ListBoxFiles lists the mock box's files in dir that match pattern.
func (c *Client) ListBoxFiles(ctx context.Context, dir, pattern string) ([]string, error) {
	if c.NoBoxFiles || c.NoFileListing {
		return nil, fmt.Errorf("mock: %w", webcontrol.ErrNotFound)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for p := range c.boxFiles() {
		if path.Dir(p) != strings.TrimSuffix(dir, "/") {
			continue
		}
		if ok, _ := path.Match(pattern, path.Base(p)); ok {
			out = append(out, path.Base(p))
		}
	}
	sort.Strings(out)
	return out, nil
}

// boxFiles is the files, made on first use; the caller holds c.mu.
func (c *Client) boxFiles() map[string][]byte {
	if c.Files == nil {
		c.Files = mockBoxFiles()
	}
	return c.Files
}
