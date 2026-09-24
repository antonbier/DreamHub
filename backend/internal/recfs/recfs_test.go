package recfs

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
)

const dir = "/media/hdd/movie"

// A real recording's full sidecar set (the #11 live test), a neighbour whose
// name starts with the same text, a non-recording, and a subfolder.
func sample() *Mem {
	return NewMem(map[string]string{
		dir + "/Crazy Rich.ts":           "video",
		dir + "/Crazy Rich.ts.ap":        "ap",
		dir + "/Crazy Rich.ts.cuts":      "c",
		dir + "/Crazy Rich.ts.meta":      "1:0:1:…\nCrazy Rich Asians\ndesc\n1780944521\n",
		dir + "/Crazy Rich.ts.sc":        "sc",
		dir + "/Crazy Rich.eit":          "eit",
		dir + "/Crazy Rich.jpg":          "jpg",
		dir + "/Crazy Rich.backdrop.jpg": "bd",
		dir + "/Crazy Rich.2.ts":         "another recording",
		dir + "/Crazy Rich.2.ts.meta":    "x\nCrazy Rich 2\n",
		dir + "/Oskars Kleid.mkv":        "not a recording",
		dir + "/Serie/Folge 1.ts":        "v",
	})
}

func TestFiles_TakesTheSidecarsNotTheNeighbour(t *testing.T) {
	got, err := Files(sample(), dir+"/Crazy Rich.ts")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		dir + "/Crazy Rich.ts", dir + "/Crazy Rich.backdrop.jpg", dir + "/Crazy Rich.eit", dir + "/Crazy Rich.jpg",
		dir + "/Crazy Rich.ts.ap", dir + "/Crazy Rich.ts.cuts", dir + "/Crazy Rich.ts.meta", dir + "/Crazy Rich.ts.sc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Files() =\n%v\nwant\n%v", got, want)
	}
	if _, err := Files(sample(), dir+"/Oskars Kleid.mkv"); !errors.Is(err, ErrNotARecording) {
		t.Errorf("a .mkv is not a recording, got %v", err)
	}
}

func TestPlanRenameAndApply(t *testing.T) {
	fsys := sample()
	moves, err := PlanRename(fsys, dir+"/Crazy Rich.ts", "Crazy Rich Asians (2018)")
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 8 || moves[len(moves)-1].To != dir+"/Crazy Rich Asians (2018).ts" {
		t.Fatalf("expected 8 moves, main file last: %+v", moves)
	}
	for _, m := range moves {
		if m.From == dir+"/Crazy Rich.2.ts" {
			t.Fatal("the neighbouring recording must not be renamed")
		}
	}
	before := len(fsys.Paths())
	if err := Apply(fsys, moves); err != nil {
		t.Fatal(err)
	}
	if len(fsys.Paths()) != before {
		t.Error("a rename must not change the number of files")
	}
	for _, p := range []string{dir + "/Crazy Rich Asians (2018).backdrop.jpg", dir + "/Crazy Rich Asians (2018).ts.meta", dir + "/Crazy Rich.2.ts"} {
		if _, err := fsys.Lstat(p); err != nil {
			t.Errorf("%s should exist after the rename", p)
		}
	}
}

func TestPlanRename_Rejects(t *testing.T) {
	fsys := sample()
	for name, want := range map[string]error{
		"":                                   ErrInvalidName,
		"a/b":                                ErrInvalidName,
		"..":                                 ErrInvalidName,
		".hidden":                            ErrInvalidName,
		"bad\nname":                          ErrInvalidName,
		"Crazy Rich":                         ErrSameName,
		"Crazy Rich.2":                       ErrTargetExists, // would collide with the neighbour
		string(make([]byte, MaxNameBytes+1)): ErrInvalidName,
	} {
		if _, err := PlanRename(fsys, dir+"/Crazy Rich.ts", name); !errors.Is(err, want) {
			t.Errorf("PlanRename(%q): got %v, want %v", name, err, want)
		}
	}
}

// failingFS fails the Nth rename, to prove all-or-nothing.
type failingFS struct {
	*Mem
	failAt, n int
}

func (f *failingFS) Rename(from, to string) error {
	f.n++
	if f.n == f.failAt {
		return fs.ErrPermission
	}
	return f.Mem.Rename(from, to)
}

func TestApply_RollsBackOnFailure(t *testing.T) {
	mem := sample()
	before := mem.Paths()
	fsys := &failingFS{Mem: mem, failAt: 4}
	moves, _ := PlanRename(fsys, dir+"/Crazy Rich.ts", "Neu")
	if err := Apply(fsys, moves); err == nil {
		t.Fatal("expected the failure to surface")
	}
	if got := mem.Paths(); !reflect.DeepEqual(got, before) {
		t.Errorf("a failed rename must leave every file as it was:\n%v\nwant\n%v", got, before)
	}
}

func TestTrashRestoreEmpty(t *testing.T) {
	fsys := sample()
	m, err := PlanTrash(fsys, dir+"/Crazy Rich.ts")
	if err != nil {
		t.Fatal(err)
	}
	if m.To != dir+"/Crazy Rich.ts.trash" {
		t.Fatalf("AMS convention: only the main file gets .trash, got %+v", m)
	}
	if err := Apply(fsys, []Move{m}); err != nil {
		t.Fatal(err)
	}

	items, err := ListTrash(fsys, []string{dir})
	if err != nil || len(items) != 1 {
		t.Fatalf("expected one trashed recording, got %+v, %v", items, err)
	}
	if items[0].Title != "Crazy Rich Asians" || items[0].OriginalPath != dir+"/Crazy Rich.ts" || items[0].Size == 0 {
		t.Errorf("unexpected trash item: %+v", items[0])
	}

	r, err := PlanRestore(fsys, items[0].Path)
	if err != nil || r.To != dir+"/Crazy Rich.ts" {
		t.Fatalf("restore: %+v, %v", r, err)
	}

	del, err := PlanEmpty(fsys, items[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(del) != 8 || del[len(del)-1] != dir+"/Crazy Rich.ts.trash" {
		t.Fatalf("emptying deletes the 7 sidecars, then the trashed file: %v", del)
	}
	if err := Remove(fsys, del); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Lstat(dir + "/Crazy Rich.2.ts"); err != nil {
		t.Error("the neighbouring recording must survive emptying the trash")
	}
	if items, _ := ListTrash(fsys, []string{dir}); len(items) != 0 {
		t.Errorf("trash should be empty, got %+v", items)
	}
}

func TestClean_KeepsPathsInsideTheFolders(t *testing.T) {
	roots := []string{"/media/hdd/movie/"}
	if p, err := Clean("//media/hdd/movie/Serie/Folge 1.ts", roots); err != nil || p != "/media/hdd/movie/Serie/Folge 1.ts" {
		t.Errorf("box path: %q, %v", p, err)
	}
	for _, p := range []string{"/media/hdd/movie/../../etc/passwd", "/etc/enigma2/settings", "/media/hdd/movie"} {
		if _, err := Clean(p, roots); !errors.Is(err, ErrOutsideFolders) {
			t.Errorf("%q must be rejected, got %v", p, err)
		}
	}
}

func TestAvailable(t *testing.T) {
	if !Available(sample(), []string{"/media/hdd/movie/"}) {
		t.Error("folders exist on this FS: available")
	}
	if Available(sample(), []string{"/nowhere/"}) || Available(nil, []string{dir}) {
		t.Error("missing folders or no FS: unavailable")
	}
}
