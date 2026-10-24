package webcontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
)

// BoxFileReader reads small files of the box through /file (the read side of
// docs/NETCAST_PLAN.md, N3: Netcast's source files in /etc/enigma2). Only
// reads; DreamHub writes no box file this way. /file?action=download answers
// on both box kinds (ARCHITECTURE.md); the directory listing is OpenWebif's
// and is not confirmed on a Dreambox, so callers have a fallback.
type BoxFileReader interface {
	// ReadBoxFile returns the file's content (an error wrapping ErrNotFound
	// when it is not there, ErrTooLarge past MaxBoxFile bytes).
	ReadBoxFile(ctx context.Context, path string) ([]byte, error)
	// ListBoxFiles returns the names of the files in dir that match the
	// glob pattern ("netcast*.json").
	ListBoxFiles(ctx context.Context, dir, pattern string) ([]string, error)
}

// MaxBoxFile caps what ReadBoxFile returns: these are small JSON files.
const MaxBoxFile = 1 << 20

// ErrTooLarge: the file is bigger than MaxBoxFile.
var ErrTooLarge = fmt.Errorf("webcontrol: the file is too large")

func (c *HTTPClient) ReadBoxFile(ctx context.Context, p string) ([]byte, error) {
	body, err := c.get(ctx, "/file?"+url.Values{"action": {"download"}, "file": {p}}.Encode())
	if err != nil {
		return nil, err
	}
	defer body.Close()
	b, err := io.ReadAll(io.LimitReader(body, MaxBoxFile+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxBoxFile {
		return nil, ErrTooLarge
	}
	return b, nil
}

func (c *HTTPClient) ListBoxFiles(ctx context.Context, dir, pattern string) ([]string, error) {
	body, err := c.get(ctx, "/file?"+url.Values{"dir": {dir}, "pattern": {pattern}}.Encode())
	if err != nil {
		return nil, err
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, MaxBoxFile))
	if err != nil {
		return nil, err
	}
	return ParseFileList(raw, pattern)
}

// ParseFileList reads OpenWebif's directory answer ({"files": [...]}, each a
// name, a path or an object with a path or name) and keeps what matches the
// pattern, as names, sorted.
func ParseFileList(raw []byte, pattern string) ([]string, error) {
	var v struct {
		Files []json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("webcontrol: reading a directory listing: %w", err)
	}
	var names []string
	for _, f := range v.Files {
		var s string
		if json.Unmarshal(f, &s) != nil {
			var o struct{ Path, Name, Filename string }
			if json.Unmarshal(f, &o) != nil {
				continue
			}
			s = firstNonEmpty(o.Path, o.Name, o.Filename)
		}
		name := path.Base(strings.TrimSpace(s))
		if ok, _ := path.Match(pattern, name); ok && name != "." && name != "/" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
