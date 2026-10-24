package iptv

import (
	"context"
	"encoding/json"
	"net/url"
	"path"
	"sort"
	"strings"
)

// GitHub repositories as a source of lists (docs/NETCAST_PLAN.md, N5): the
// person pastes a repository or folder address, DreamHub lists the guide and
// playlist files in it through the public contents API and uses
// raw.githubusercontent.com addresses. Public repositories only.

// GitHubAPI is the contents API's base; tests point it elsewhere.
var GitHubAPI = "https://api.github.com"

// RawHost serves the files.
const RawHost = "raw.githubusercontent.com"

// Repo is a place in a repository.
type Repo struct {
	Owner, Name, Ref, Dir string // Ref "" means the default branch
}

// ParseRepo understands https://github.com/owner/repo, …/tree/<ref>/<dir> and
// "owner/repo". A branch name with a slash cannot be told from a folder; it
// is read as the first segment only.
func ParseRepo(s string) (Repo, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "/")
	var parts []string
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || (u.Host != "github.com" && u.Host != "www.github.com") {
			return Repo{}, false
		}
		parts = strings.Split(strings.Trim(u.Path, "/"), "/")
	} else {
		parts = strings.Split(s, "/")
	}
	if len(parts) < 2 {
		return Repo{}, false
	}
	r := Repo{Owner: parts[0], Name: strings.TrimSuffix(parts[1], ".git")}
	if !validRepoPart(r.Owner) || !validRepoPart(r.Name) {
		return Repo{}, false
	}
	rest := parts[2:]
	if len(rest) > 0 {
		if (rest[0] != "tree" && rest[0] != "blob") || len(rest) < 2 {
			return Repo{}, false
		}
		r.Ref = rest[1]
		r.Dir = strings.Join(rest[2:], "/")
		if rest[0] == "blob" && r.Dir != "" {
			r.Dir = path.Dir(r.Dir)
			if r.Dir == "." {
				r.Dir = ""
			}
		}
	}
	return r, true
}

func validRepoPart(s string) bool {
	if s == "" || len(s) > 100 || s == "." || s == ".." {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// RepoFile is a list file found in a repository.
type RepoFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"` // "guide" or "playlist"
	Size int64  `json:"size"`
	URL  string `json:"-"` // raw address
}

// RepoListing is what a folder holds.
type RepoListing struct {
	Files []RepoFile `json:"files"`
	Dirs  []string   `json:"dirs"` // sub-folders, to look further
}

func fileKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".xml"), strings.HasSuffix(n, ".xml.gz"):
		return "guide"
	case strings.HasSuffix(n, ".m3u"), strings.HasSuffix(n, ".m3u8"), strings.HasSuffix(n, ".m3u.gz"):
		return "playlist"
	}
	return ""
}

// ListRepo lists the guide and playlist files directly in a folder.
func (f *Fetcher) ListRepo(ctx context.Context, r Repo) (*RepoListing, error) {
	api := GitHubAPI + "/repos/" + r.Owner + "/" + r.Name + "/contents"
	if r.Dir != "" {
		api += "/" + escapePath(r.Dir)
	}
	if r.Ref != "" {
		api += "?ref=" + url.QueryEscape(r.Ref)
	}
	raw, err := f.FetchLimited(ctx, api, 2<<20)
	if err != nil {
		return nil, err
	}
	var items []struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Type        string `json:"type"`
		Size        int64  `json:"size"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		// a single file comes back as an object: not a folder
		return nil, &FetchError{Kind: KindNotPlaylist}
	}
	out := &RepoListing{Files: []RepoFile{}, Dirs: []string{}}
	for _, it := range items {
		switch it.Type {
		case "dir":
			out.Dirs = append(out.Dirs, it.Path)
		case "file":
			k := fileKind(it.Name)
			if k == "" {
				continue
			}
			ref := r.Ref
			if ref == "" {
				ref = "HEAD"
			}
			out.Files = append(out.Files, RepoFile{Name: it.Name, Path: it.Path, Kind: k, Size: it.Size,
				URL: "https://" + RawHost + "/" + r.Owner + "/" + r.Name + "/" + url.PathEscape(ref) + "/" + escapePath(it.Path)})
		}
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	sort.Strings(out.Dirs)
	return out, nil
}

func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
