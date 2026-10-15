package hubs

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// Where an app's files go: one of the box's disks, chosen at install. The
// box's own flash is deliberately never offered — a database on it would
// wear it out, and a full one stops the box (docs/APPSHUB_PLAN.md,
// "Storage: HDD by default, flash is a hard safety line").

// StorageChoice is one place an app can be stored.
type StorageChoice struct {
	// Path is the storage root: apps get one folder each below it.
	Path string `json:"path"`
	// Label is what to call it — the disk's mount point.
	Label     string `json:"label"`
	FreeBytes int64  `json:"freeBytes"`
	Default   bool   `json:"default,omitempty"`
}

// minStorageFreeBytes: a location with less free than this can't be chosen.
const minStorageFreeBytes = 1 << 30

// boxPorts are the ports the box itself uses; an app can never be moved to
// one (DreamHub 8080, its Docker helper 8090, WebControl 80, …).
var boxPorts = map[int]bool{80: true, 443: true, 8080: true, 8090: true, 8001: true, 4200: true, 139: true, 445: true}

// storageFS are the file system types a database can live on safely; FAT,
// NTFS and network shares can't hold a container's file ownership or
// locking.
var storageFS = map[string]bool{"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "f2fs": true}

// rawStorageChoices are the places apps can be stored, before the chosen
// default is marked.
func (h *Apps) rawStorageChoices(ctx context.Context) []StorageChoice {
	if h.Storage != nil {
		return h.Storage(ctx)
	}
	return mountedStorage(h.StorageRoot)
}

// storageChoices are the places apps can be stored, the one new apps go to
// by default (the Docker setting, else the recording disk) marked.
func (h *Apps) storageChoices(ctx context.Context) []StorageChoice {
	choices := h.rawStorageChoices(ctx)
	def := h.defaultRoot(ctx)
	out := make([]StorageChoice, len(choices))
	for i, c := range choices {
		c.Default = c.Path == def
		out[i] = c
	}
	return out
}

// mountedStorage lists the writable disks mounted directly under /media,
// the default root first (always offered, even if it isn't on a mount of
// its own).
func mountedStorage(def string) []StorageChoice {
	var out []StorageChoice
	free := func(dir string) int64 {
		for d := dir; d != "/" && d != "."; d = path.Dir(d) { // the root may not exist yet
			if n, ok := freeBytesAt(d); ok {
				return n
			}
		}
		return 0
	}
	if def != "" {
		out = append(out, StorageChoice{Path: def, Label: path.Dir(def), FreeBytes: free(def), Default: true})
	}
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		mount, fstype, opts := fields[1], fields[2], strings.Split(fields[3], ",")
		if path.Dir(mount) != "/media" || !storageFS[fstype] {
			continue
		}
		ro := false
		for _, o := range opts {
			ro = ro || o == "ro"
		}
		root := path.Join(mount, "dreamhub-apps")
		if ro || root == def {
			continue
		}
		out = append(out, StorageChoice{Path: root, Label: mount, FreeBytes: free(mount)})
	}
	return out
}

// storage answers GET /api/v1/apps/storage.
func (h *Apps) storage(w http.ResponseWriter, r *http.Request) {
	choices := h.storageChoices(r.Context())
	if choices == nil {
		choices = []StorageChoice{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"choices": choices})
}
