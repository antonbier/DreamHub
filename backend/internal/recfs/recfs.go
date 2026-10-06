// Package recfs does MediaHub's file operations on recordings: rename, move
// to trash, restore, empty the trash. WebControl has no endpoint that fits
// (ARCHITECTURE.md: moviedelete is a hard delete, moviemove can't rename and
// leaves .backdrop.jpg behind), so this works on the file system directly —
// which only makes sense when DreamHub runs on the box itself.
//
// Rules, from the device findings:
//   - A recording is a ".ts" file plus its sidecars, all in one folder:
//     "<base>.ts", "<base>.ts.*" (.meta, .ap, .sc, .cuts, …), "<base>.eit",
//     cover art "<base>.jpg" / "<base>.backdrop.jpg" / "<base>.png".
//     Deliberately not "everything starting with <base>" (AdvancedMovieSelection's
//     glob): that would also catch "<base>.2.ts", a different recording.
//   - Trash follows AdvancedMovieSelection's convention so the TV's own
//     wastebasket and DreamHub agree: only the main file is renamed to
//     "<base>.ts.trash"; the sidecars stay; emptying deletes all of them.
//     DreamHub implements it itself — AMS is optional.
//   - A multi-file change is all-or-nothing: on the first failure, what was
//     already renamed is renamed back.
//   - Paths come from the box's own listing and must lie inside one of its
//     recording folders; symlinks are never followed or touched.
package recfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// FS is the file system recfs works on: the real one (OS) or Mem.
type FS interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	Lstat(name string) (fs.FileInfo, error)
	Rename(from, to string) error
	Remove(name string) error
	ReadFile(name string) ([]byte, error)
	// WriteFile replaces an existing file's content atomically (a temporary
	// file next to it, then a rename), keeping its permissions.
	WriteFile(name string, data []byte) error
	// Mkdir creates one folder (its parent must exist).
	Mkdir(name string) error
	// Create writes a new file; it fails when anything exists at name
	// already, so it can never overwrite (or write through a link).
	Create(name string, data []byte) error
}

// Opener is an FS that opens a file for reading in place — StreamHub
// plays recordings from it, seeking by byte range, without reading them
// whole. OS and Mem have it.
type Opener interface {
	Open(name string) (io.ReadSeekCloser, error)
}

// OS is the real file system.
type OS struct{}

func (OS) Open(name string) (io.ReadSeekCloser, error) { return os.Open(name) }

func (OS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }
func (OS) Lstat(name string) (fs.FileInfo, error)     { return os.Lstat(name) }
func (OS) Rename(from, to string) error               { return os.Rename(from, to) }
func (OS) Remove(name string) error                   { return os.Remove(name) }
func (OS) ReadFile(name string) ([]byte, error)       { return os.ReadFile(name) }

// Mkdir creates a folder with the usual mode for recording folders (0755,
// set explicitly: DreamHub runs with umask 077, and the box's other
// readers — a Samba share, say — need to see into it).
func (OS) Mkdir(name string) error {
	if err := os.Mkdir(name, 0o755); err != nil {
		return err
	}
	return os.Chmod(name, 0o755)
}

// Create writes a new file with mode 0644 (set explicitly, as Mkdir), via
// O_EXCL: an existing file or link at name is an error, never replaced.
// A failed write leaves nothing behind.
func (OS) Create(name string, data []byte) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(name, 0o644)
	}
	if err != nil {
		_ = os.Remove(name)
	}
	return err
}

func (OS) WriteFile(name string, data []byte) error {
	fi, err := os.Lstat(name)
	if err != nil {
		return err
	}
	tmp := name + ".dreamhub-tmp"
	if err := os.WriteFile(tmp, data, fi.Mode().Perm()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Explicitly: DreamHub runs with umask 077, which WriteFile would apply,
	// and the box's other readers (a Samba share, say) expect the old mode.
	if err := os.Chmod(tmp, fi.Mode().Perm()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Error codes (i18n keys for the client, under "error.media").
var (
	ErrNotARecording  = errors.New("not_a_recording") // not a .ts file, or not found
	ErrOutsideFolders = errors.New("outside_folders") // not inside a recording folder
	ErrInvalidName    = errors.New("invalid_name")    // empty, a path, too long, …
	ErrSameName       = errors.New("same_name")       // nothing would change
	ErrTargetExists   = errors.New("target_exists")   // a file with the new name exists
	ErrNotInTrash     = errors.New("not_in_trash")    // not a ".ts.trash" file
	ErrRestoreBlocked = errors.New("restore_blocked") // the original name is taken again
)

// TrashSuffix is AdvancedMovieSelection's trash marker.
const TrashSuffix = ".trash"

// MaxNameBytes keeps "<base>" short enough that its longest sidecar
// ("<base>.backdrop.jpg", "<base>.ts.trash" …) stays under ext4's 255-byte
// file name limit.
const MaxNameBytes = 200

// Move is one file rename.
type Move struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Clean normalizes a path from the box ("//media/hdd/movie/x.ts") and
// checks it lies inside one of the recording folders.
func Clean(p string, roots []string) (string, error) {
	p = path.Clean("/" + strings.TrimLeft(p, "/"))
	for _, r := range roots {
		r = path.Clean("/" + strings.TrimLeft(r, "/"))
		if strings.HasPrefix(p, r+"/") {
			return p, nil
		}
	}
	return "", ErrOutsideFolders
}

// CleanIn is Clean plus a check against symbolic links: every folder
// between the recording folder and p must be a real folder, not a link. A
// link inside the recording folders (e.g. "movie/x -> /etc") would
// otherwise let a file operation reach outside them while the path still
// looks inside.
func CleanIn(fsys FS, p string, roots []string) (string, error) {
	p, err := Clean(p, roots)
	if err != nil {
		return "", err
	}
	for _, r := range roots {
		root := path.Clean("/" + strings.TrimLeft(r, "/"))
		if !strings.HasPrefix(p, root+"/") {
			continue
		}
		for dir := path.Dir(p); dir != root && strings.HasPrefix(dir, root+"/"); dir = path.Dir(dir) {
			fi, err := fsys.Lstat(dir)
			if err != nil {
				return "", err
			}
			if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
				return "", ErrOutsideFolders
			}
		}
		return p, nil
	}
	return "", ErrOutsideFolders
}

// Base returns a recording's base name: "…/X.ts" → "X".
func Base(tsPath string) string { return strings.TrimSuffix(path.Base(tsPath), ".ts") }

// belongs reports whether file name is part of the recording with this base.
func belongs(name, base string) bool {
	if name == base+".ts" || strings.HasPrefix(name, base+".ts.") {
		return true
	}
	switch name {
	case base + ".eit", base + ".jpg", base + ".backdrop.jpg", base + ".png":
		return true
	}
	return false
}

// Covers returns a recording's cover art already on disk — written by
// another plugin next to the recording (confirmed on the reference box:
// "<base>.jpg" and "<base>.backdrop.jpg", ARCHITECTURE.md #11) — as paths,
// "" where there is none. Regular files only, never a link.
func Covers(fsys FS, tsPath string) (poster, backdrop string) {
	dir, base := path.Dir(tsPath), Base(tsPath)
	regular := func(p string) bool {
		fi, err := fsys.Lstat(p)
		return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
	}
	for _, name := range []string{base + ".jpg", base + ".png"} {
		if p := path.Join(dir, name); regular(p) {
			poster = p
			break
		}
	}
	if p := path.Join(dir, base+".backdrop.jpg"); regular(p) {
		backdrop = p
	}
	return poster, backdrop
}

// Files returns every regular file of the recording at tsPath (which must
// exist and be a regular .ts file), main file first.
func Files(fsys FS, tsPath string) ([]string, error) {
	if !strings.HasSuffix(tsPath, ".ts") {
		return nil, ErrNotARecording
	}
	if fi, err := fsys.Lstat(tsPath); err != nil || !fi.Mode().IsRegular() {
		return nil, ErrNotARecording
	}
	return siblings(fsys, path.Dir(tsPath), Base(tsPath), tsPath)
}

func siblings(fsys FS, dir, base, main string) ([]string, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []string{main}
	for _, e := range entries {
		full := path.Join(dir, e.Name())
		if full == main || !e.Type().IsRegular() || !belongs(e.Name(), base) {
			continue
		}
		out = append(out, full)
	}
	return out, nil
}

// ValidName checks a new base name and returns it trimmed.
func ValidName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".ts")
	switch {
	case name == "", name == ".", name == "..", strings.HasPrefix(name, "."),
		strings.ContainsAny(name, "/\x00"), !utf8.ValidString(name), len(name) > MaxNameBytes:
		return "", ErrInvalidName
	}
	for _, r := range name {
		if r < 0x20 { // control characters
			return "", ErrInvalidName
		}
	}
	return name, nil
}

// PlanRename lists the moves that rename the recording at tsPath to newBase,
// in the order they'll be applied (main file last). Nothing is changed.
func PlanRename(fsys FS, tsPath, newBase string) ([]Move, error) {
	return PlanRenameTo(fsys, tsPath, path.Dir(tsPath), newBase)
}

// PlanRenameTo is PlanRename into another folder newDir (which may not
// exist yet — see MissingDirs): every file of the recording moves along.
func PlanRenameTo(fsys FS, tsPath, newDir, newBase string) ([]Move, error) {
	newBase, err := ValidName(newBase)
	if err != nil {
		return nil, err
	}
	files, err := Files(fsys, tsPath)
	if err != nil {
		return nil, err
	}
	oldBase := Base(tsPath)
	newDir = path.Clean(newDir)
	if newBase == oldBase && newDir == path.Dir(tsPath) {
		return nil, ErrSameName
	}
	moves := make([]Move, 0, len(files))
	for _, f := range files[1:] {
		moves = append(moves, Move{From: f, To: path.Join(newDir, newBase+strings.TrimPrefix(path.Base(f), oldBase))})
	}
	moves = append(moves, Move{From: tsPath, To: path.Join(newDir, newBase+".ts")})
	for _, m := range moves {
		if _, err := fsys.Lstat(m.To); err == nil {
			return nil, ErrTargetExists
		}
	}
	return moves, nil
}

// MissingDirs lists the folders (outermost first) that moves need and
// that don't exist yet, stopping at an existing folder. A path segment
// that exists but isn't a real folder (a file, a link) is an error: a move
// must never go through a link.
func MissingDirs(fsys FS, moves []Move) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, m := range moves {
		var chain []string
		for dir := path.Dir(m.To); dir != "/" && dir != "." && !seen[dir]; dir = path.Dir(dir) {
			fi, err := fsys.Lstat(dir)
			if err == nil {
				if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
					return nil, ErrOutsideFolders
				}
				break
			}
			chain = append(chain, dir)
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if !seen[chain[i]] {
				seen[chain[i]] = true
				out = append(out, chain[i])
			}
		}
	}
	return out, nil
}

// ApplyInto creates the missing folders, then applies moves — all or
// nothing: on a failure it renames back what it moved and removes the
// folders it created (only if they're empty again).
func ApplyInto(fsys FS, dirs []string, moves []Move) error {
	var made []string
	undo := func() {
		for i := len(made) - 1; i >= 0; i-- {
			_ = fsys.Remove(made[i])
		}
	}
	for _, d := range dirs {
		if err := fsys.Mkdir(d); err != nil {
			undo()
			return err
		}
		made = append(made, d)
	}
	if err := Apply(fsys, moves); err != nil {
		undo()
		return err
	}
	return nil
}

// PlanTrash moves a recording to the trash: only its main file is renamed.
func PlanTrash(fsys FS, tsPath string) (Move, error) {
	if _, err := Files(fsys, tsPath); err != nil {
		return Move{}, err
	}
	to := tsPath + TrashSuffix
	if _, err := fsys.Lstat(to); err == nil {
		return Move{}, ErrTargetExists
	}
	return Move{From: tsPath, To: to}, nil
}

// PlanRestore takes a recording back out of the trash.
func PlanRestore(fsys FS, trashPath string) (Move, error) {
	if !strings.HasSuffix(trashPath, ".ts"+TrashSuffix) {
		return Move{}, ErrNotInTrash
	}
	if fi, err := fsys.Lstat(trashPath); err != nil || !fi.Mode().IsRegular() {
		return Move{}, ErrNotInTrash
	}
	to := strings.TrimSuffix(trashPath, TrashSuffix)
	if _, err := fsys.Lstat(to); err == nil {
		return Move{}, ErrRestoreBlocked
	}
	return Move{From: trashPath, To: to}, nil
}

// PlanEmpty lists the files deleting a trashed recording removes for good:
// its sidecars, then the trashed main file.
func PlanEmpty(fsys FS, trashPath string) ([]string, error) {
	if !strings.HasSuffix(trashPath, ".ts"+TrashSuffix) {
		return nil, ErrNotInTrash
	}
	if fi, err := fsys.Lstat(trashPath); err != nil || !fi.Mode().IsRegular() {
		return nil, ErrNotInTrash
	}
	original := strings.TrimSuffix(trashPath, TrashSuffix)
	files, err := siblings(fsys, path.Dir(trashPath), Base(original), trashPath)
	if err != nil {
		return nil, err
	}
	// Main (trash) file last, so an interrupted run still shows up in the
	// trash rather than leaving orphaned sidecars behind unnoticed.
	return append(files[1:], files[0]), nil
}

// Apply performs moves in order. On the first failure it renames back what
// it already moved and returns the error: all or nothing.
func Apply(fsys FS, moves []Move) error {
	for i, m := range moves {
		if _, err := fsys.Lstat(m.To); err == nil {
			rollback(fsys, moves[:i])
			return ErrTargetExists
		}
		if err := fsys.Rename(m.From, m.To); err != nil {
			rollback(fsys, moves[:i])
			return err
		}
	}
	return nil
}

func rollback(fsys FS, done []Move) {
	for i := len(done) - 1; i >= 0; i-- {
		_ = fsys.Rename(done[i].To, done[i].From)
	}
}

// Remove deletes files in order, stopping at the first error.
func Remove(fsys FS, files []string) error {
	for _, f := range files {
		if err := fsys.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// TrashItem is one recording in the trash.
type TrashItem struct {
	Path         string    `json:"path"`         // the ".ts.trash" file
	OriginalPath string    `json:"originalPath"` // where it goes on restore
	Title        string    `json:"title"`        // from .ts.meta, else the base name
	Size         int64     `json:"size"`         // bytes, all of its files
	Modified     time.Time `json:"modified"`     // the recording file's mtime
}

// maxDepth bounds the walk (the reference box nests 4 deep at most).
const maxDepth = 8

// ListTrash finds every trashed recording below roots, newest first.
func ListTrash(fsys FS, roots []string) ([]TrashItem, error) {
	var out []TrashItem
	seen := map[string]bool{}
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if depth > maxDepth || seen[dir] {
			return nil
		}
		seen[dir] = true
		entries, err := fsys.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			full := path.Join(dir, e.Name())
			switch {
			case e.IsDir(): // DirEntry of a symlink is never IsDir, so links aren't followed
				if err := walk(full, depth+1); err != nil && !errors.Is(err, fs.ErrPermission) {
					return err
				}
			case e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".ts"+TrashSuffix):
				item := trashItem(fsys, full)
				out = append(out, item)
			}
		}
		return nil
	}
	for _, r := range roots {
		if err := walk(path.Clean(r), 0); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out, nil
}

func trashItem(fsys FS, trashPath string) TrashItem {
	original := strings.TrimSuffix(trashPath, TrashSuffix)
	item := TrashItem{Path: trashPath, OriginalPath: original, Title: Base(original)}
	if fi, err := fsys.Lstat(trashPath); err == nil {
		item.Modified = fi.ModTime().UTC()
	}
	if files, err := siblings(fsys, path.Dir(trashPath), Base(original), trashPath); err == nil {
		for _, f := range files {
			if fi, err := fsys.Lstat(f); err == nil {
				item.Size += fi.Size()
			}
		}
	}
	if title := MetaTitle(fsys, original); title != "" {
		item.Title = title
	}
	return item
}

// MetaTitle reads a recording's title from line 2 of its .ts.meta
// (ARCHITECTURE.md: line 1 service reference, line 2 title, …), or "".
func MetaTitle(fsys FS, tsPath string) string {
	data, err := fsys.ReadFile(tsPath + ".meta")
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 3)
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimSpace(lines[1])
}

// ErrNoMeta means a recording has no .ts.meta whose title could change.
var ErrNoMeta = errors.New("no_meta")

// ErrInvalidTitle means a title that can't go into .ts.meta: empty, too
// long or spanning lines (each line of the file is its own field).
var ErrInvalidTitle = errors.New("invalid_title")

// ValidTitle trims a title for .ts.meta and checks it.
func ValidTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 200 || strings.ContainsAny(title, "\r\n") || !utf8.ValidString(title) {
		return "", ErrInvalidTitle
	}
	return title, nil
}

// SetMetaTitle replaces line 2 of a recording's .ts.meta — the title
// Enigma2's own movie list shows (ARCHITECTURE.md: line 1 service
// reference, line 2 title, line 3 description, line 4 time) — leaving every
// other line exactly as it was. Returns the previous content, for undoing.
func SetMetaTitle(fsys FS, tsPath, title string) ([]byte, error) {
	title, err := ValidTitle(title)
	if err != nil {
		return nil, err
	}
	// A regular file only: a .ts.meta that is a link to some other file
	// must not have that file's content copied into the recording folder.
	if fi, err := fsys.Lstat(tsPath + ".meta"); err != nil || !fi.Mode().IsRegular() {
		return nil, ErrNoMeta
	}
	old, err := fsys.ReadFile(tsPath + ".meta")
	if err != nil {
		return nil, ErrNoMeta
	}
	lines := strings.Split(string(old), "\n")
	if len(lines) < 2 {
		return nil, ErrNoMeta
	}
	lines[1] = title
	if err := fsys.WriteFile(tsPath+".meta", []byte(strings.Join(lines, "\n"))); err != nil {
		return nil, err
	}
	return old, nil
}

// Available reports whether the recording folders exist on this file system
// — i.e. whether DreamHub runs on the box itself. Off the box (development
// against a remote device) file operations are unavailable, not broken.
func Available(fsys FS, roots []string) bool {
	if fsys == nil || len(roots) == 0 {
		return false
	}
	for _, r := range roots {
		fi, err := fsys.Lstat(path.Clean(r))
		if err != nil || !fi.IsDir() {
			return false
		}
	}
	return true
}
