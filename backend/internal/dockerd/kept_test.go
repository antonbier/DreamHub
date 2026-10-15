package dockerd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// imageDaemon is a small stateful stand-in for the Docker daemon: images
// with names, containers that run an image, a pull that moves a name to a
// new image, tag and delete the way the real API does. The rollback paths
// need that (a name that moves is the whole point), which the canned
// answers of fakeDaemon can't show.
type imageDaemon struct {
	mu         sync.Mutex
	images     map[string]*fakeImage
	containers map[string]*fakeContainer
	pulls      int
	failPull   bool
}

type fakeImage struct {
	tags map[string]bool
	size int64
}

type fakeContainer struct {
	name, image, imageRef string
	labels                map[string]string
	restarts              int
}

func (d *imageDaemon) holder(ref string) string {
	if !strings.Contains(ref[strings.LastIndex(ref, "/")+1:], ":") {
		ref += ":latest"
	}
	for id, i := range d.images {
		if i.tags[ref] {
			return id
		}
	}
	return ""
}

func (d *imageDaemon) resolve(ref string) string {
	if _, ok := d.images[ref]; ok {
		return ref
	}
	return d.holder(ref)
}

func (d *imageDaemon) tag(id, ref string) {
	for _, i := range d.images {
		delete(i.tags, ref)
	}
	d.images[id].tags[ref] = true
}

func (d *imageDaemon) used(id string) bool {
	for _, c := range d.containers {
		if c.image == id {
			return true
		}
	}
	return false
}

// removeIfBare deletes an image with no name left and no container.
func (d *imageDaemon) removeIfBare(id string) bool {
	if len(d.images[id].tags) == 0 && !d.used(id) {
		delete(d.images, id)
		return true
	}
	return false
}

func newImageDaemon(t *testing.T) (*imageDaemon, Exec) {
	t.Helper()
	d := &imageDaemon{images: map[string]*fakeImage{}, containers: map[string]*fakeContainer{}}
	sock := filepath.Join(t.TempDir(), "docker.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	mux.HandleFunc("GET /containers/json", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		var out []map[string]any
		for id, c := range d.containers {
			out = append(out, map[string]any{"Id": id, "Names": []string{"/" + c.name}, "Image": c.imageRef, "ImageID": c.image, "State": "running", "Labels": c.labels})
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["Id"].(string) < out[j]["Id"].(string) })
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /containers/{id}/json", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		id := r.PathValue("id")
		for cid, c := range d.containers {
			if cid == id || c.name == id {
				writeJSON(w, map[string]any{"Image": c.image, "State": map[string]any{"RestartCount": c.restarts},
					"Config": map[string]any{"Image": c.imageRef, "Labels": c.labels}})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("POST /images/create", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.failPull {
			_, _ = w.Write([]byte(`{"error":"toomanyrequests"}` + "\n"))
			return
		}
		d.pulls++
		id := fmt.Sprintf("sha256:pulled%d", d.pulls)
		d.images[id] = &fakeImage{tags: map[string]bool{}, size: 100}
		ref := r.URL.Query().Get("fromImage")
		if !strings.Contains(ref[strings.LastIndex(ref, "/")+1:], ":") {
			ref += ":latest"
		}
		old := d.holder(ref)
		d.tag(id, ref)
		_ = old // the old image keeps its other names; with none it is dangling
		_, _ = w.Write([]byte(`{"status":"done"}` + "\n"))
	})
	mux.HandleFunc("POST /containers/create", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		var body struct {
			Image  string
			Labels map[string]string
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := d.resolve(body.Image)
		if id == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		cid := "c" + fmt.Sprint(len(d.containers)+1+d.pulls*10)
		d.containers[cid] = &fakeContainer{name: r.URL.Query().Get("name"), image: id, imageRef: body.Image, labels: body.Labels}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"Id": cid})
	})
	mux.HandleFunc("POST /containers/{id}/start", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /containers/{id}/stop", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("DELETE /containers/{id}", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		delete(d.containers, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	// Images: the names have slashes, so one handler reads the path.
	mux.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		rest := strings.TrimPrefix(r.URL.Path, "/images/")
		switch {
		case r.Method == "GET" && rest == "json":
			var out []map[string]any
			for id, i := range d.images {
				var tags []string
				for t := range i.tags {
					tags = append(tags, t)
				}
				sort.Strings(tags)
				out = append(out, map[string]any{"Id": id, "RepoTags": tags, "Size": i.size, "SharedSize": 0})
			}
			sort.Slice(out, func(a, b int) bool { return out[a]["Id"].(string) < out[b]["Id"].(string) })
			writeJSON(w, out)
		case r.Method == "GET" && strings.HasSuffix(rest, "/json"):
			id := d.resolve(strings.TrimSuffix(rest, "/json"))
			if id == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var tags []string
			for t := range d.images[id].tags {
				tags = append(tags, t)
			}
			writeJSON(w, map[string]any{"Id": id, "RepoTags": tags})
		case r.Method == "POST" && strings.HasSuffix(rest, "/tag"):
			id := d.resolve(strings.TrimSuffix(rest, "/tag"))
			if id == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			d.tag(id, r.URL.Query().Get("repo")+":"+r.URL.Query().Get("tag"))
			w.WriteHeader(http.StatusCreated)
		case r.Method == "DELETE":
			id := d.resolve(rest)
			if id == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var out []map[string]string
			if rest == id { // by id: every name goes, if nothing runs from it
				if d.used(id) {
					w.WriteHeader(http.StatusConflict)
					return
				}
				for t := range d.images[id].tags {
					out = append(out, map[string]string{"Untagged": t})
				}
				delete(d.images, id)
				out = append(out, map[string]string{"Deleted": id})
			} else { // by name: only that name, the image with it when it was the last
				delete(d.images[id].tags, rest)
				out = append(out, map[string]string{"Untagged": rest})
				if d.removeIfBare(id) {
					out = append(out, map[string]string{"Deleted": id})
				}
			}
			writeJSON(w, out)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv := &httptest.Server{Listener: l, Config: &http.Server{Handler: mux}}
	srv.Start()
	t.Cleanup(srv.Close)
	return d, Exec{SocketPath: sock}
}

func (d *imageDaemon) names(id string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	if i := d.images[id]; i != nil {
		for t := range i.tags {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

func (d *imageDaemon) imageOfContainer(name string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range d.containers {
		if c.name == name {
			return c.image
		}
	}
	return ""
}

func spec(image string, fromKept bool) ContainerSpec {
	return ContainerSpec{AppID: "pihole", Image: image, HostNetwork: true, HostPort: 80, ContainerPort: 80, MemoryLimitBytes: 1 << 28, FromKept: fromKept}
}

// seed makes the state of an installed app: image A under the app's name,
// running.
func (d *imageDaemon) seed() {
	d.images["sha256:A"] = &fakeImage{tags: map[string]bool{"pihole/pihole:latest": true}, size: 1000}
	d.containers["c0"] = &fakeContainer{name: "dreamhub-app-pihole", image: "sha256:A", imageRef: "pihole/pihole:latest", labels: map[string]string{AppLabel: "pihole"}}
}

func TestExec_UpdateKeepsThePreviousImageAndRollsBack(t *testing.T) {
	d, e := newImageDaemon(t)
	d.seed()
	ctx := context.Background()

	id, err := e.KeepImage(ctx, "c0")
	if err != nil || id != "sha256:A" {
		t.Fatalf("KeepImage = %q, %v", id, err)
	}
	// The update: the container goes, the pull moves the name to image B.
	if err := e.Remove(ctx, "c0"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Create(ctx, spec("pihole/pihole:latest", false)); err != nil {
		t.Fatal(err)
	}
	if got := d.imageOfContainer("dreamhub-app-pihole"); got != "sha256:pulled1" {
		t.Fatalf("the update runs %s", got)
	}
	// A is not running, has no ordinary name any more, but is kept.
	kept, err := e.KeptImages(ctx)
	if err != nil || len(kept) != 1 || kept[0].AppID != "pihole" || kept[0].ImageID != "sha256:A" {
		t.Fatalf("kept = %+v, %v", kept, err)
	}
	// ... and the clean-up neither lists nor removes it.
	unused, _ := e.UnusedImages(ctx)
	for _, i := range unused {
		if i.ID == "sha256:A" {
			t.Fatalf("the kept image is listed as unused: %+v", unused)
		}
	}
	if _, err := e.PruneImages(ctx); err != nil {
		t.Fatal(err)
	}
	if len(d.names("sha256:A")) == 0 {
		t.Fatal("the clean-up removed the rollback image")
	}

	// Roll back: the new container goes, the old image gets its name back.
	if err := e.Remove(ctx, "c11"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Create(ctx, spec("pihole/pihole:latest", true)); err != nil {
		t.Fatal(err)
	}
	if got := d.imageOfContainer("dreamhub-app-pihole"); got != "sha256:A" {
		t.Fatalf("after the roll back the app runs %s, want the old image", got)
	}
	// Nothing to roll back to any more once the person drops it.
	res, err := e.DropKeptImages(ctx, "pihole")
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 0 { // the image is in use again: only its rollback names went
		t.Errorf("drop removed %d images, the running one must stay", res.Removed)
	}
	if kept, _ := e.KeptImages(ctx); len(kept) != 0 {
		t.Errorf("still kept: %+v", kept)
	}
	if names := d.names("sha256:A"); len(names) != 1 || names[0] != "pihole/pihole:latest" {
		t.Errorf("the old image's names: %v", names)
	}
}

func TestExec_FromKeptNeedsAKeptImageAndItsOwnName(t *testing.T) {
	d, e := newImageDaemon(t)
	d.seed()
	ctx := context.Background()
	if _, err := e.Create(ctx, ContainerSpec{AppID: "other", Image: "x/y:1", HostNetwork: true, HostPort: 1, ContainerPort: 1, MemoryLimitBytes: 1, FromKept: true}); !errors.Is(err, ErrNoKeptImage) {
		t.Fatalf("nothing kept: %v", err)
	}
	if _, err := e.KeepImage(ctx, "c0"); err != nil {
		t.Fatal(err)
	}
	_ = e.Remove(ctx, "c0")
	// The kept image can come back under the name it had, not under any
	// other (that would let the call rename an image at will).
	_, err := e.Create(ctx, spec("library/nginx:latest", true))
	var se *SpecError
	if !errors.As(err, &se) {
		t.Fatalf("another name: %v", err)
	}
	if d.holder("library/nginx:latest") != "" {
		t.Fatal("the image got an arbitrary name")
	}
}

func TestExec_KeepIsOneLevel(t *testing.T) {
	d, e := newImageDaemon(t)
	d.seed()
	ctx := context.Background()
	_, _ = e.KeepImage(ctx, "c0")
	_ = e.Remove(ctx, "c0")
	_, _ = e.Create(ctx, spec("pihole/pihole:latest", false)) // B
	// A second update: B is kept now, A gives up its rollback names.
	_, err := e.KeepImage(ctx, "c11")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.names("sha256:A"); len(got) != 0 {
		t.Errorf("the first kept image still has names: %v", got)
	}
	kept, _ := e.KeptImages(ctx)
	if len(kept) != 1 || kept[0].ImageID != "sha256:pulled1" {
		t.Fatalf("kept = %+v", kept)
	}
	// Nothing that is not an app's own container can be kept.
	if _, err := e.KeepImage(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown container: %v", err)
	}
}

func TestExec_DropRemovesAnUnusedKeptImage(t *testing.T) {
	d, e := newImageDaemon(t)
	d.seed()
	ctx := context.Background()
	_, _ = e.KeepImage(ctx, "c0")
	_ = e.Remove(ctx, "c0")
	_, _ = e.Create(ctx, spec("pihole/pihole:latest", false))
	res, err := e.DropKeptImages(ctx, "")
	if err != nil || res.Removed != 1 || res.ReclaimedBytes != 1000 {
		t.Fatalf("drop = %+v, %v", res, err)
	}
	if _, ok := d.images["sha256:A"]; ok {
		t.Error("the unused kept image is still on the disk")
	}
	if _, err := e.DropKeptImages(ctx, "Bad ID"); err == nil {
		t.Error("an invalid app id must be refused")
	}
}

func TestExec_ListReadsRestartCount(t *testing.T) {
	d, e := newImageDaemon(t)
	d.seed()
	d.containers["c0"].restarts = 4
	list, err := e.List(context.Background())
	if err != nil || len(list) != 1 || list[0].RestartCount != 4 {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

func TestExec_FailedPullLeavesTheOldNameInPlace(t *testing.T) {
	d, e := newImageDaemon(t)
	d.seed()
	d.failPull = true
	ctx := context.Background()
	_, _ = e.KeepImage(ctx, "c0")
	_ = e.Remove(ctx, "c0")
	if _, err := e.Create(ctx, spec("pihole/pihole:latest", false)); err == nil {
		t.Fatal("the pull failed, Create must too")
	}
	// The name still points to the old image: it can be started again.
	if got := d.holder("pihole/pihole:latest"); got != "sha256:A" {
		t.Fatalf("name now on %q", got)
	}
}
