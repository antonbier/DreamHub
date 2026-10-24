package webcontrol

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	// ReadBoxFileMax is ReadBoxFile with its own size limit, for a list that
	// is bigger than a settings file (Netcast's cached playlists).
	ReadBoxFileMax(ctx context.Context, path string, max int64) ([]byte, error)
	// ListBoxFiles returns the names of the files in dir that match the
	// glob pattern ("netcast*.json").
	ListBoxFiles(ctx context.Context, dir, pattern string) ([]string, error)
}

// BoxFileWriter writes a small file on the box. Neither WebControl nor
// OpenWebif has a call for it: it needs a write route in the DreamHub TV
// plugin (box settings S3), so the real client answers ErrNeedsPlugin until
// that exists. The writer is told nothing about what the file is for; the
// plan that uses it reads the file back and keeps a copy of the old one.
type BoxFileWriter interface {
	// CanWriteBoxFiles says whether the box has the write route.
	CanWriteBoxFiles(ctx context.Context) bool
	WriteBoxFile(ctx context.Context, path string, data []byte) error
}

// MaxBoxFile caps what ReadBoxFile returns: these are small JSON files.
const MaxBoxFile = 1 << 20

// ErrTooLarge: the file is bigger than MaxBoxFile.
var ErrTooLarge = fmt.Errorf("webcontrol: the file is too large")

func (c *HTTPClient) ReadBoxFile(ctx context.Context, p string) ([]byte, error) {
	return c.ReadBoxFileMax(ctx, p, MaxBoxFile)
}

func (c *HTTPClient) ReadBoxFileMax(ctx context.Context, p string, max int64) ([]byte, error) {
	body, err := c.get(ctx, "/file?"+url.Values{"action": {"download"}, "file": {p}}.Encode())
	if err != nil {
		return nil, err
	}
	defer body.Close()
	b, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, ErrTooLarge
	}
	return b, nil
}

func (c *HTTPClient) ListBoxFiles(ctx context.Context, dir, pattern string) ([]string, error) {
	body, err := c.get(ctx, "/file?"+url.Values{"dir": {dir}, "pattern": {pattern}}.Encode())
	if errors.Is(err, ErrNotFound) {
		// A Dreambox's WebControl has no /file listing; its media player's
		// listing shows any folder (confirmed on the One, 2026-10-24).
		return c.listViaMediaPlayer(ctx, dir, pattern)
	}
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

// listViaMediaPlayer lists dir through /web/mediaplayerlist. The call glues
// the folder and the file name together without a slash
// ("/etc/enigma2netcast_x.json"), and without a `types` filter shows only
// media files, so the pattern's extension is passed as the filter.
func (c *HTTPClient) listViaMediaPlayer(ctx context.Context, dir, pattern string) ([]string, error) {
	dir = strings.TrimRight(dir, "/")
	q := url.Values{"path": {dir}}
	if ext := path.Ext(pattern); ext != "" && !strings.ContainsAny(ext, "*?[") {
		q.Set("types", strings.TrimPrefix(ext, "."))
	}
	body, err := c.get(ctx, "/web/mediaplayerlist?"+q.Encode())
	if err != nil {
		return nil, err
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, 4*MaxBoxFile))
	if err != nil {
		return nil, err
	}
	return ParseMediaPlayerList(raw, dir, pattern)
}

// ParseMediaPlayerList reads /web/mediaplayerlist (e2filelist) and keeps the
// files of dir that match the pattern, as names, sorted.
func ParseMediaPlayerList(raw []byte, dir, pattern string) ([]string, error) {
	var v struct {
		Files []struct {
			Ref   string `xml:"e2servicereference"`
			IsDir string `xml:"e2isdirectory"`
		} `xml:"e2file"`
	}
	if err := xml.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("webcontrol: reading a directory listing: %w", err)
	}
	var names []string
	for _, f := range v.Files {
		if strings.EqualFold(strings.TrimSpace(f.IsDir), "true") {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(f.Ref), dir)
		name = strings.TrimPrefix(name, "/")
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		if ok, _ := path.Match(pattern, name); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
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

// ErrWritesOff: the box has the write route but the switch in DreamHub's
// settings on the TV is off (config.plugins.dreamhub.fileWrite).
var ErrWritesOff = errors.New("webcontrol: file writing is turned off in DreamHub's settings on the box")

const dreamboxFilesPath = "/dreamhub/files/"

// filesInfo asks the plugin's file route what it can do; ErrNeedsPlugin when
// the box has none (no DreamHub plugin, or one older than 0.9.5).
func (c *HTTPClient) filesInfo(ctx context.Context) (writes bool, err error) {
	body, err := c.get(ctx, dreamboxFilesPath+"info")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, fmt.Errorf("%w (%sinfo)", ErrNeedsPlugin, dreamboxFilesPath)
		}
		return false, err
	}
	defer body.Close()
	var v struct {
		OK     bool `json:"ok"`
		API    int  `json:"api"`
		Writes bool `json:"writes"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<16)).Decode(&v); err != nil || !v.OK || v.API < 1 {
		return false, fmt.Errorf("%w (%sinfo)", ErrNeedsPlugin, dreamboxFilesPath)
	}
	return v.Writes, nil
}

// WriteBoxFile replaces one of Netcast's source lists through the DreamHub
// plugin's /dreamhub/files/write (enigma2-plugin/src/webfiles.py): a POST with
// the custom header, refused unless the switch on the TV is on. The plugin
// takes only netcast*_*.json in /etc/enigma2 (and the .dreamhub-backup copy).
func (c *HTTPClient) WriteBoxFile(ctx context.Context, p string, data []byte) error {
	writes, err := c.filesInfo(ctx)
	if err != nil {
		return err
	}
	if !writes {
		return ErrWritesOff
	}
	defer c.wrote()
	body, err := c.doWithHeader(ctx, http.MethodPost, dreamboxFilesPath+"write",
		url.Values{"path": {p}, "data": {string(data)}}, http.Header{"X-Dreamhub-Request": {"1"}})
	if err != nil {
		return err
	}
	defer body.Close()
	var res struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<16)).Decode(&res); err != nil {
		return fmt.Errorf("webcontrol: %swrite: %w", dreamboxFilesPath, err)
	}
	if res.OK {
		return nil
	}
	if res.Error == "writes_off" {
		return ErrWritesOff
	}
	return &RejectedError{Op: "write file", Text: res.Error + ": " + res.Message}
}

// CanWriteBoxFiles: the plugin has the route and the switch is on.
func (c *HTTPClient) CanWriteBoxFiles(ctx context.Context) bool {
	writes, err := c.filesInfo(ctx)
	return err == nil && writes
}
