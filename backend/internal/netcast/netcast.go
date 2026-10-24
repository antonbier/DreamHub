// Package netcast reads the source files of the Gemini Netcast plugin
// (netcastserver_*.json and netcastepg_*.json in /etc/enigma2) into what
// DreamHub shows (docs/NETCAST_PLAN.md, N3). Read only. The layout is the
// plugin README's on a Dreambox One with Netcast 2.4: a file holds a list of
// sources (usually under "serverlist" or "epglist") and sometimes settings
// next to it. Fields this package does not know are left out, and anything
// that could be a login never leaves it: a source only says "has a login".
package netcast

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/iptv"
)

// Dir is where the plugin keeps its files.
const Dir = "/etc/enigma2"

// Patterns of the two kinds of file.
const (
	ServerPattern = "netcastserver_*.json"
	EPGPattern    = "netcastepg_*.json"
)

// KnownFiles are the file names seen on the reference box, tried when the box
// cannot list a directory.
var KnownFiles = []string{"netcastserver_gemini.json", "netcastserver_plutotv.json", "netcastepg_gemini.json"}

// Source is one entry of a file.
type Source struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType,omitempty"` // playlist, playlist_live, playlist_radio, api, …
	Address   string `json:"address,omitempty"`   // without login, query and fragment
	HasLogin  bool   `json:"hasLogin,omitempty"`  // a user name or password, or one in the address
	Desc      string `json:"desc,omitempty"`
}

// File is a parsed source file.
type File struct {
	Name     string            `json:"name"`
	Kind     string            `json:"kind"` // "server" or "epg"
	Sources  []Source          `json:"sources"`
	Settings map[string]string `json:"settings,omitempty"` // plain values next to the list, such as EPGrefreshHour
}

// KindOf says what kind of file a name is, or "".
func KindOf(name string) string {
	switch {
	case strings.HasPrefix(name, "netcastserver_") && strings.HasSuffix(name, ".json"):
		return "server"
	case strings.HasPrefix(name, "netcastepg_") && strings.HasSuffix(name, ".json"):
		return "epg"
	}
	return ""
}

var addressKeys = []string{"mainUrl", "epgUrl", "url", "playlistUrl", "epgurl", "mainurl"}

func isSecretKey(k string) bool {
	k = strings.ToLower(k)
	for _, w := range []string{"user", "pass", "token", "key", "secret", "auth", "login", "cookie"} {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return fmt.Sprint(x)
	case bool:
		return fmt.Sprint(x)
	}
	return ""
}

// Parse reads one file's content.
func Parse(name string, raw []byte) (*File, error) {
	kind := KindOf(name)
	if kind == "" {
		return nil, fmt.Errorf("netcast: %q is not a Netcast source file", name)
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("netcast: %s is not valid JSON", name)
	}
	f := &File{Name: name, Kind: kind, Sources: []Source{}}
	keys := make([]string, 0, len(top))
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch v := top[k].(type) {
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					f.Sources = append(f.Sources, source(m))
				}
			}
		case string, float64, bool:
			if !isSecretKey(k) {
				if f.Settings == nil {
					f.Settings = map[string]string{}
				}
				f.Settings[k] = str(v)
			}
		}
	}
	return f, nil
}

func source(m map[string]any) Source {
	s := Source{ID: str(m["id"]), Name: str(m["name"]), MediaType: str(m["mediatype"]), Desc: str(m["desc"])}
	for k, v := range m {
		if isSecretKey(k) && str(v) != "" {
			s.HasLogin = true
		}
	}
	for _, k := range addressKeys {
		if a := str(m[k]); a != "" {
			s.Address = iptv.Display(a)
			s.HasLogin = s.HasLogin || iptv.HasLogin(a)
			break
		}
	}
	if s.Name == "" {
		s.Name = s.ID
	}
	return s
}
