package netcast

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// Editing a source file (docs/NETCAST_PLAN.md, N3 and N5). The file is read
// into a generic map and written back, so every field this package does not
// know — a login, `pic`, `UpdateTime`, `UserAgent` — survives an edit. Two
// things do change: the keys come out sorted and the file is indented,
// because JSON in Go has no key order to keep. The plugin reads JSON, not
// text.

// Input is what the person gives for a source. Empty fields are left as they
// are when updating.
type Input struct {
	ID        string
	Name      string
	Address   string // the playlist or guide address; may carry a login, never shown back
	MediaType string // servers only
	Desc      string // servers only
}

// Errors of the editing functions.
var (
	ErrInvalid  = errors.New("netcast: invalid source")
	ErrExists   = errors.New("netcast: a source with that id exists")
	ErrNoSource = errors.New("netcast: no such source")
	ErrBadFile  = errors.New("netcast: the file is not a source list")
)

// MediaTypes the plugin knows for a server entry (ARCHITECTURE.md).
var MediaTypes = []string{"playlist", "playlist_vod", "playlist_live", "playlist_radio"}

func listKey(kind string) string {
	if kind == "epg" {
		return "entrys"
	}
	return "serverlist"
}

func addressKey(kind string) string {
	if kind == "epg" {
		return "url"
	}
	return "mainUrl"
}

// Slug makes an id from a name: lower-case letters and digits, prefixed "dh"
// so it cannot collide with the plugin's own ids.
func Slug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue // not ASCII: leave it out, ids go into addresses
		}
	}
	if b.Len() == 0 {
		return ""
	}
	s := b.String()
	if len(s) > 24 {
		s = s[:24]
	}
	return "dh" + s
}

// FileName is the file a new source of that id goes into.
func FileName(kind, id string) string {
	if kind == "epg" {
		return "netcastepg_" + id + ".json"
	}
	return "netcastserver_" + id + ".json"
}

func validate(kind string, in Input, adding bool) error {
	if kind != "server" && kind != "epg" {
		return fmt.Errorf("%w: kind", ErrInvalid)
	}
	if adding && (strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Address) == "") {
		return fmt.Errorf("%w: name and address are required", ErrInvalid)
	}
	if len([]rune(in.Name)) > 80 || len([]rune(in.Desc)) > 200 {
		return fmt.Errorf("%w: too long", ErrInvalid)
	}
	if a := strings.TrimSpace(in.Address); a != "" {
		u, err := url.Parse(a)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(a, " \t\r\n\"") {
			return fmt.Errorf("%w: address", ErrInvalid)
		}
	}
	if kind == "server" && in.MediaType != "" {
		ok := false
		for _, m := range MediaTypes {
			ok = ok || m == in.MediaType
		}
		if !ok {
			return fmt.Errorf("%w: media type", ErrInvalid)
		}
	}
	return nil
}

type doc struct {
	top  map[string]any
	list []any
	key  string
}

// open reads raw; an empty raw is a new file.
func open(kind string, raw []byte) (*doc, error) {
	d := &doc{top: map[string]any{}, key: listKey(kind)}
	if len(bytes.TrimSpace(raw)) == 0 {
		return d, nil
	}
	if err := json.Unmarshal(raw, &d.top); err != nil {
		return nil, ErrBadFile
	}
	// the list is the array under the kind's key, else the only array of objects
	if l, ok := d.top[d.key].([]any); ok {
		d.list = l
		return d, nil
	}
	found := ""
	for k, v := range d.top {
		if l, ok := v.([]any); ok {
			if found != "" {
				return nil, ErrBadFile
			}
			found, d.list = k, l
		}
	}
	if found != "" {
		d.key = found
	}
	return d, nil
}

func (d *doc) bytes() []byte {
	d.top[d.key] = d.list
	out, _ := json.MarshalIndent(d.top, "", "  ")
	return append(out, '\n')
}

func (d *doc) find(id string) int {
	for i, e := range d.list {
		if m, ok := e.(map[string]any); ok && str(m["id"]) == id {
			return i
		}
	}
	return -1
}

// Add appends a source to a file's content (raw may be empty: a new file).
// The id is the input's, else made from the name.
func Add(kind string, raw []byte, in Input) (content []byte, id string, err error) {
	if err := validate(kind, in, true); err != nil {
		return nil, "", err
	}
	d, err := open(kind, raw)
	if err != nil {
		return nil, "", err
	}
	id = strings.TrimSpace(in.ID)
	if id == "" {
		id = Slug(in.Name)
	}
	if id == "" {
		return nil, "", fmt.Errorf("%w: id", ErrInvalid)
	}
	if d.find(id) >= 0 {
		return nil, "", ErrExists
	}
	e := map[string]any{"id": id, "name": strings.TrimSpace(in.Name), addressKey(kind): strings.TrimSpace(in.Address)}
	if kind == "server" {
		mt := in.MediaType
		if mt == "" {
			mt = "playlist_live"
		}
		e["mediatype"] = mt
		if in.Desc != "" {
			e["desc"] = in.Desc
		}
	}
	d.list = append(d.list, e)
	return d.bytes(), id, nil
}

// Update changes the given fields of the source with that id.
func Update(kind string, raw []byte, id string, in Input) ([]byte, error) {
	if err := validate(kind, in, false); err != nil {
		return nil, err
	}
	d, err := open(kind, raw)
	if err != nil {
		return nil, err
	}
	i := d.find(id)
	if i < 0 {
		return nil, ErrNoSource
	}
	e := d.list[i].(map[string]any)
	if v := strings.TrimSpace(in.Name); v != "" {
		e["name"] = v
	}
	if v := strings.TrimSpace(in.Address); v != "" {
		e[addressKey(kind)] = v
	}
	if kind == "server" {
		if in.MediaType != "" {
			e["mediatype"] = in.MediaType
		}
		if in.Desc != "" {
			e["desc"] = in.Desc
		}
	}
	return d.bytes(), nil
}

// Remove takes the source with that id out. A file left with no source is
// kept with an empty list (DreamHub deletes no file).
func Remove(kind string, raw []byte, id string) ([]byte, error) {
	d, err := open(kind, raw)
	if err != nil {
		return nil, err
	}
	i := d.find(id)
	if i < 0 {
		return nil, ErrNoSource
	}
	d.list = append(d.list[:i:i], d.list[i+1:]...)
	return d.bytes(), nil
}

// Find returns the source with that id from a file's content.
func Find(name string, raw []byte, id string) (Source, bool) {
	f, err := Parse(name, raw)
	if err != nil {
		return Source{}, false
	}
	for _, s := range f.Sources {
		if s.ID == id {
			return s, true
		}
	}
	return Source{}, false
}
