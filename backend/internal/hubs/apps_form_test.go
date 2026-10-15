package hubs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/compose"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
)

// The mount rules on a real directory tree, with symlinks: the checks run
// on what the paths really are, not on how they are written.
func TestCheckMounts_SymlinksAndProtectedData(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(tmp, "media")
	root := filepath.Join(media, "hdd", "dreamhub-apps")
	data := filepath.Join(media, "hdd", "dreamhub-data")
	for _, d := range []string{filepath.Join(root, "mytool"), filepath.Join(media, "hdd", "movie"), data, filepath.Join(tmp, "elsewhere")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{
		filepath.Join(media, "hdd", "escape"):  filepath.Join(tmp, "elsewhere"),
		filepath.Join(media, "hdd", "toetc"):   "/etc",
		filepath.Join(media, "hdd", "todata"):  data,
		filepath.Join(media, "hdd", "storage"): root,
		filepath.Join(media, "hdd", "ok"):      filepath.Join(media, "hdd", "movie"),
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	h := &Apps{StorageRoot: root, Policy: &dockerd.MountPolicy{Bases: []string{media}, Protected: []string{data}}}

	build := func(host string) appcatalog.App {
		t.Helper()
		f, err := compose.Parse([]byte("services:\n app:\n  image: traefik/whoami\n  volumes:\n   - " + quote(host+":/x") + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		app, err := appcatalog.Resolve("mytool", f, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		return app
	}
	cases := []struct {
		name, host, want string
	}{
		{"a plain media folder", filepath.Join(media, "hdd", "movie"), ""},
		{"a link inside media to a media folder", filepath.Join(media, "hdd", "ok"), ""},
		{"a link out of the media tree", filepath.Join(media, "hdd", "escape"), "mount_outside"},
		{"a link to /etc", filepath.Join(media, "hdd", "toetc"), "mount_system"},
		{"a link into /etc, a new subfolder", filepath.Join(media, "hdd", "toetc", "new"), "mount_system"},
		{"DreamHub's data", data, "mount_dreamhub"},
		{"a link to DreamHub's data", filepath.Join(media, "hdd", "todata"), "mount_dreamhub"},
		{"a link to the apps' storage", filepath.Join(media, "hdd", "storage"), "mount_storage"},
		{"the disk that holds DreamHub's data", filepath.Join(media, "hdd"), "mount_dreamhub"},
		{"dot dot", filepath.Join(media, "hdd", "movie") + "/../../../etc", "mount_unclean"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ferr := h.checkMounts(build(c.host), root)
			got := ""
			if ferr != nil {
				got = ferr.details.(map[string]any)["reason"].(string)
			}
			if got != c.want {
				t.Fatalf("%s: got %q, want %q", c.host, got, c.want)
			}
		})
	}
}

func quote(s string) string { return `"` + s + `"` }

// A viewer-facing detail: the helper's refusal reasons are what the UI keys
// its sentences on; every reason the rules can give has a code.
func TestHostDirOf(t *testing.T) {
	root := "/media/hdd/dreamhub-apps"
	for _, c := range []struct {
		v       compose.Volume
		service string
		want    string
	}{
		{compose.Volume{}, "", root + "/app"},
		{compose.Volume{}, "db", root + "/app.db"},
		{compose.Volume{Name: "data"}, "", root + "/app.data"},
		{compose.Volume{Name: "data"}, "db", root + "/app.data"},
		{compose.Volume{HostPath: "/media/x"}, "", "/media/x"},
	} {
		if got := hostDirOf(root, "app", c.service, c.v); got != c.want {
			t.Errorf("%+v in %q: %s, want %s", c.v, c.service, got, c.want)
		}
	}
}

// The containers made for a curated app are what they were before App12:
// helper first, storage layout <root>/<id>, <root>/<id>.<service>,
// <root>/<id>.data, memory per container, the host network.
func TestAppSpecs_CuratedLayoutUnchanged(t *testing.T) {
	h := &Apps{}
	root := "/media/hdd/dreamhub-apps"
	nc, _ := appcatalog.ByID("nextcloud")
	specs := h.appSpecs(nc, 512, "Europe/Rome", root, false, defaultMemoryLimitMB)
	if len(specs) != 2 || specs[0].Service != "db" || specs[1].Service != "" {
		t.Fatalf("order: %+v", specs)
	}
	db, main := specs[0], specs[1]
	if len(db.Mounts) != 1 || db.Mounts[0].HostDir != root+"/nextcloud.db" || db.Mounts[0].ContainerDir != "/var/lib/mysql" || db.MemoryLimitBytes != 256<<20 {
		t.Fatalf("db: %+v", db)
	}
	if len(main.Mounts) != 2 || main.Mounts[0].HostDir != root+"/nextcloud" || main.Mounts[0].ContainerDir != "/var/www/html" ||
		main.Mounts[1].HostDir != root+"/nextcloud.data" || main.Mounts[1].ContainerDir != "/var/www/html/data" || main.MemoryLimitBytes != 512<<20 {
		t.Fatalf("main: %+v", main)
	}
	if !main.HostNetwork || !db.HostNetwork || main.Root != root || main.HostPort != 8088 || main.ContainerPort != 8088 || db.HostPort != 0 {
		t.Fatalf("network/ports: %+v %+v", main, db)
	}
	if main.Env["TZ"] != "Europe/Rome" || main.Env["MYSQL_PASSWORD"] != "{{secret:db}}" || db.Env["MARIADB_ROOT_PASSWORD"] != "{{secret:dbroot}}" {
		t.Fatalf("env: %v / %v", main.Env, db.Env)
	}
	if main.Elevated.Any() || db.Elevated.Any() || main.Name != "" {
		t.Fatal("a curated app is a plain container without a custom name")
	}
	qb, _ := appcatalog.ByID("qbittorrent")
	specs = h.appSpecs(qb, 256, "UTC", root, false, defaultMemoryLimitMB)
	if len(specs) != 1 || specs[0].Mounts[1].HostDir != root+"/qbittorrent.data" || specs[0].Mounts[1].ContainerDir != "/downloads" {
		t.Fatalf("qbittorrent: %+v", specs)
	}
}
