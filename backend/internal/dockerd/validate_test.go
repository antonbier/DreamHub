package dockerd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRoot = "/media/hdd/dreamhub-apps"

// lexicalPolicy decides on the path text only: no file system access.
func lexicalPolicy() MountPolicy {
	return MountPolicy{
		Bases:     []string{"/media"},
		Protected: []string{"/media/hdd/dreamhub-data"},
		Resolve:   func(s string) (string, error) { return s, nil },
		Exists:    func(s string) bool { return s == "/media/usb/dreamhub-apps" },
	}
}

func code(err error) string {
	if se, ok := IsRefusal(err); ok {
		return se.Code
	}
	if err == nil {
		return ""
	}
	return "other: " + err.Error()
}

func TestCheckMount_Table(t *testing.T) {
	cases := []struct {
		name, root, host, want string // want "" = allowed
	}{
		// allowed
		{"own folder", testRoot, testRoot + "/nextcloud", ""},
		{"own helper folder", testRoot, testRoot + "/nextcloud.db", ""},
		{"own data folder, deeper", testRoot, testRoot + "/nextcloud.data/sub/dir", ""},
		{"media subfolder", testRoot, "/media/hdd/movie", ""},
		{"another disk", testRoot, "/media/usb/stuff", ""},
		{"usb stick wholesale", testRoot, "/media/usb2", ""},

		// the system
		{"root", testRoot, "/", "mount_system"},
		{"etc", testRoot, "/etc", "mount_system"},
		{"inside etc", testRoot, "/etc/ssh", "mount_system"},
		{"usr", testRoot, "/usr", "mount_system"},
		{"usr lib", testRoot, "/usr/lib", "mount_system"},
		{"var", testRoot, "/var", "mount_system"},
		{"var lib", testRoot, "/var/lib/dreamhub", "mount_system"},
		{"var run docker sock", testRoot, "/var/run/docker.sock", "mount_system"},
		{"boot", testRoot, "/boot", "mount_system"},
		{"proc", testRoot, "/proc", "mount_system"},
		{"proc self", testRoot, "/proc/1/root", "mount_system"},
		{"sys", testRoot, "/sys", "mount_system"},
		{"dev", testRoot, "/dev", "mount_system"},
		{"dev sda", testRoot, "/dev/sda", "mount_system"},
		{"bin", testRoot, "/bin", "mount_system"},
		{"run", testRoot, "/run/docker.sock", "mount_system"},
		{"root home", testRoot, "/root", "mount_system"},
		{"data (flash)", testRoot, "/data", "mount_system"},
		{"dreamhub data dir default", testRoot, "/data/dreamhub", "mount_system"},
		{"data outside the app's storage", testRoot, "/data/anything", "mount_system"},
		{"opt", testRoot, "/opt/x", "mount_system"},

		// the socket
		{"docker.sock under media", testRoot, "/media/hdd/docker.sock", "mount_socket"},
		{"renamed sock", testRoot, "/media/x/my-docker.sock", "mount_socket"},

		// DreamHub's own data
		{"dreamhub data under media", testRoot, "/media/hdd/dreamhub-data", "mount_dreamhub"},
		{"inside dreamhub data", testRoot, "/media/hdd/dreamhub-data/db.sqlite", "mount_dreamhub"},

		// other apps' storage and the storage itself
		{"the storage folder", testRoot, testRoot, "mount_storage"},
		{"another app's folder", testRoot, testRoot + "/pihole", "mount_storage"},
		{"an app with a similar name", testRoot, testRoot + "/nextcloud2", "mount_storage"},
		{"own passwords file", testRoot, testRoot + "/nextcloud.secrets.json", "mount_storage"},
		{"the disk holding DreamHub's data", testRoot, "/media/hdd", "mount_dreamhub"},
		{"a disk holding another storage", testRoot, "/media/usb", "mount_storage"},
		{"inside another storage root", testRoot, "/media/usb/dreamhub-apps/pihole", "mount_storage"},
		{"no storage root given", "", testRoot + "/nextcloud", "mount_storage"},

		// outside the allowed places
		{"media holds DreamHub's data below it", testRoot, "/media", "mount_dreamhub"},
		{"tmp", testRoot, "/tmp/x", "mount_outside"},
		{"mnt", testRoot, "/mnt/x", "mount_outside"},
		{"look-alike prefix", testRoot, "/mediafoo/x", "mount_outside"},

		// not clean
		{"empty", testRoot, "", "mount_unclean"},
		{"relative", testRoot, "media/hdd/x", "mount_unclean"},
		{"dot dot out", testRoot, "/media/../etc", "mount_unclean"},
		{"dot dot inside", testRoot, "/media/hdd/../usb/x", "mount_unclean"},
		{"trailing slash", testRoot, "/media/hdd/x/", "mount_unclean"},
		{"double slash", testRoot, "/media//x", "mount_unclean"},
		{"colon", testRoot, "/media/x:/etc", "mount_unclean"},
		{"comma", testRoot, "/media/x,y", "mount_unclean"},
		{"newline", testRoot, "/media/x\n/etc", "mount_unclean"},
		{"nul", testRoot, "/media/x\x00", "mount_unclean"},
	}
	p := lexicalPolicy()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := code(p.CheckMount(c.root, "nextcloud", c.host))
			if got != c.want {
				t.Fatalf("CheckMount(%q, %q) = %q, want %q", c.root, c.host, got, c.want)
			}
		})
	}
}

// Without protected data under /media, the disk and /media itself are
// refused for their own reasons.
func TestCheckMount_DiskAndMedia(t *testing.T) {
	p := lexicalPolicy()
	p.Protected = nil
	for host, want := range map[string]string{"/media/hdd": "mount_storage", "/media": "mount_storage", testRoot: "mount_storage"} {
		if got := code(p.CheckMount(testRoot, "nextcloud", host)); got != want {
			t.Errorf("%s: %q, want %q", host, got, want)
		}
	}
}

// A root that isn't strictly below a base gives the app no own folders.
func TestCheckMount_RootOutsideBases(t *testing.T) {
	p := lexicalPolicy()
	for _, root := range []string{"/", "/etc/apps", "/tmp/apps", "/data/dreamhub-apps"} {
		if err := p.CheckMount(root, "nextcloud", root+"/nextcloud"); err == nil {
			t.Errorf("root %q: its own folder was allowed", root)
		}
	}
}

// Symlinks and .. are decided on what the paths really are.
func TestCheckMount_Symlinks(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(tmp, "media")
	root := filepath.Join(media, "hdd", "dreamhub-apps")
	mustMkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustLink := func(target, link string) {
		t.Helper()
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	mustMkdir(filepath.Join(root, "nextcloud"))
	mustMkdir(filepath.Join(root, "pihole"))
	mustMkdir(filepath.Join(media, "hdd", "movie"))
	mustMkdir(filepath.Join(tmp, "outside"))
	mustMkdir(filepath.Join(tmp, "dreamhubdata"))
	mustLink(filepath.Join(tmp, "outside"), filepath.Join(media, "hdd", "toutside"))
	mustLink("/etc", filepath.Join(media, "hdd", "toetc"))
	mustLink("/", filepath.Join(media, "hdd", "toroot"))
	mustLink(root, filepath.Join(media, "hdd", "tostorage"))
	mustLink(filepath.Join(root, "pihole"), filepath.Join(root, "nextcloud", "topihole"))
	mustLink(filepath.Join(tmp, "dreamhubdata"), filepath.Join(media, "hdd", "todata"))
	mustLink(filepath.Join(media, "hdd", "movie"), filepath.Join(media, "hdd", "movielink"))
	mustLink(filepath.Join(media, "hdd", "nowhere"), filepath.Join(media, "hdd", "dangling"))
	mustLink("loop2", filepath.Join(media, "hdd", "loop1"))
	mustLink("loop1", filepath.Join(media, "hdd", "loop2"))

	p := MountPolicy{Bases: []string{media}, Protected: []string{filepath.Join(tmp, "dreamhubdata")}}
	p2 := MountPolicy{Bases: []string{media}, Protected: []string{filepath.Join(media, "hdd", "dreamhubdata2")}}
	_ = p2
	j := func(parts ...string) string { return filepath.Join(append([]string{media}, parts...)...) }

	cases := []struct {
		name, host, want string
	}{
		{"real own folder", filepath.Join(root, "nextcloud"), ""},
		{"own folder not yet created", filepath.Join(root, "nextcloud.data"), ""},
		{"real media folder", j("hdd", "movie"), ""},
		{"a link to another media folder", j("hdd", "movielink"), ""},
		{"link out of the media tree", j("hdd", "toutside"), "mount_outside"},
		{"link to /etc", j("hdd", "toetc"), "mount_system"},
		{"link to /etc, deeper", j("hdd", "toetc", "ssh"), "mount_system"},
		{"link to /", j("hdd", "toroot"), "mount_system"},
		{"link to the storage", j("hdd", "tostorage"), "mount_storage"},
		{"link below the storage", j("hdd", "tostorage", "pihole"), "mount_storage"},
		{"own folder linking to another app", filepath.Join(root, "nextcloud", "topihole"), "mount_storage"},
		{"link to protected data", j("hdd", "todata"), "mount_dreamhub"},
		{"dangling link", j("hdd", "dangling"), "mount_unresolvable"},
		{"link loop", j("hdd", "loop1"), "mount_unresolvable"},
		{"not-yet-existing path below a link", j("hdd", "toetc", "newdir"), "mount_system"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := code(p.CheckMount(root, "nextcloud", c.host))
			if got != c.want {
				t.Fatalf("CheckMount(%s) = %q, want %q", c.host, got, c.want)
			}
		})
	}
	// ".." never reaches the file system: it is refused as written.
	if got := code(p.CheckMount(root, "nextcloud", media+"/hdd/movie/../../../etc")); got != "mount_unclean" {
		t.Errorf(".. traversal: %q", got)
	}
	// A storage root that is itself a link out of the base is not trusted.
	mustLink(filepath.Join(tmp, "outside"), filepath.Join(media, "usb"))
	if err := p.CheckMount(filepath.Join(media, "usb", "dreamhub-apps"), "nextcloud", filepath.Join(media, "usb", "dreamhub-apps", "nextcloud")); err == nil {
		t.Error("a storage root that resolves outside the base was trusted")
	}
}

func TestCheckDevice(t *testing.T) {
	ok := []string{"/dev/ttyUSB0", "/dev/ttyUSB0:/dev/zigbee", "/dev/dvb/adapter0/frontend0:/dev/dvb/adapter0/frontend0:rw", "/dev/sda:/dev/sda:r"}
	for _, d := range ok {
		if err := CheckDevice(d); err != nil {
			t.Errorf("%q refused: %v", d, err)
		}
	}
	bad := []string{"", "ttyUSB0", "/etc/passwd", "/dev/../etc/passwd", "/dev/x/", "/dev//x", "/dev/mem", "/dev/kmem", "/dev/port", "/dev/kmsg",
		"/dev/mmcblk0", "/dev/mmcblk0p3", "/dev/root", "/dev/mtd0", "/dev/mtdblock3", "/dev/ubi0", "/dev/x:/etc/y", "/dev/x:/dev/y:rwx", "/dev/x:/dev/y:", "/dev/a:/dev/b:rw:z", "/dev/x,/dev/y"}
	for _, d := range bad {
		if err := CheckDevice(d); err == nil {
			t.Errorf("%q was allowed", d)
		} else if code(err) != "device_refused" {
			t.Errorf("%q: %v", d, err)
		}
	}
}

func goodSpec() ContainerSpec {
	return ContainerSpec{
		AppID: "myapp", Image: "ghcr.io/x/y:1", MemoryLimitBytes: 1 << 20, Root: testRoot, HostNetwork: true,
		Mounts: []Mount{{HostDir: testRoot + "/myapp", ContainerDir: "/config"}},
	}
}

func TestValidateSpec(t *testing.T) {
	p := lexicalPolicy()
	if err := ValidateSpec(goodSpec(), p); err != nil {
		t.Fatal(err)
	}
	mut := func(f func(*ContainerSpec)) ContainerSpec { s := goodSpec(); f(&s); return s }
	cases := []struct {
		name string
		spec ContainerSpec
		want string
	}{
		{"app id upper", mut(func(s *ContainerSpec) { s.AppID = "MyApp" }), "invalid"},
		{"app id slash", mut(func(s *ContainerSpec) { s.AppID = "a/b" }), "invalid"},
		{"image option", mut(func(s *ContainerSpec) { s.Image = "--privileged" }), "invalid"},
		{"image space", mut(func(s *ContainerSpec) { s.Image = "x y" }), "invalid"},
		{"image digest", mut(func(s *ContainerSpec) { s.Image = "x@sha256:1" }), "invalid"},
		{"env key", mut(func(s *ContainerSpec) { s.Env = map[string]string{"A=B": "c"} }), "invalid"},
		{"env nul", mut(func(s *ContainerSpec) { s.Env = map[string]string{"A": "c\x00"} }), "invalid"},
		{"bad root", mut(func(s *ContainerSpec) { s.Root = "relative" }), "invalid"},
		{"mount point relative", mut(func(s *ContainerSpec) { s.Mounts[0].ContainerDir = "config" }), "invalid"},
		{"two at one point", mut(func(s *ContainerSpec) {
			s.Mounts = append(s.Mounts, Mount{HostDir: testRoot + "/myapp.data", ContainerDir: "/config"})
		}), "invalid"},
		{"socket", mut(func(s *ContainerSpec) {
			s.Mounts = append(s.Mounts, Mount{HostDir: "/var/run/docker.sock", ContainerDir: "/var/run/docker.sock"})
		}), "mount_system"},
		{"etc", mut(func(s *ContainerSpec) { s.Mounts = append(s.Mounts, Mount{HostDir: "/etc", ContainerDir: "/host"}) }), "mount_system"},
		{"privileged unconfirmed", mut(func(s *ContainerSpec) { s.Elevated.Privileged = true }), "needs_confirmation"},
		{"cap unconfirmed", mut(func(s *ContainerSpec) { s.Elevated.CapAdd = []string{"NET_ADMIN"} }), "needs_confirmation"},
		{"device unconfirmed", mut(func(s *ContainerSpec) { s.Elevated.Devices = []string{"/dev/ttyUSB0"} }), "needs_confirmation"},
		{"pid unconfirmed", mut(func(s *ContainerSpec) { s.Elevated.PidHost = true }), "needs_confirmation"},
		{"ipc unconfirmed", mut(func(s *ContainerSpec) { s.Elevated.IpcHost = true }), "needs_confirmation"},
		{"confirmed but bad cap", mut(func(s *ContainerSpec) { s.Elevated = Elevated{CapAdd: []string{"net admin"}, Confirmed: true} }), "invalid"},
		{"confirmed but forbidden device", mut(func(s *ContainerSpec) { s.Elevated = Elevated{Devices: []string{"/dev/mem"}, Confirmed: true} }), "device_refused"},
		{"confirmation does not unlock mounts", mut(func(s *ContainerSpec) {
			s.Elevated = Elevated{Privileged: true, Confirmed: true}
			s.Mounts = append(s.Mounts, Mount{HostDir: "/", ContainerDir: "/host"})
		}), "mount_system"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := code(ValidateSpec(c.spec, p)); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
	ok := mut(func(s *ContainerSpec) {
		s.Elevated = Elevated{Privileged: true, CapAdd: []string{"NET_ADMIN"}, Devices: []string{"/dev/ttyUSB0:/dev/zigbee"}, PidHost: true, IpcHost: true, Confirmed: true}
	})
	if err := ValidateSpec(ok, p); err != nil {
		t.Fatalf("a confirmed request was refused: %v", err)
	}
	if err := ValidateSpec(mut(func(s *ContainerSpec) { s.Service = "DB" }), p); !errors.Is(err, ErrInvalidService) {
		t.Errorf("service: %v", err)
	}
	many := goodSpec()
	for i := 0; i < maxMounts+1; i++ {
		many.Mounts = append(many.Mounts, Mount{HostDir: testRoot + "/myapp.d" + strings.Repeat("x", i), ContainerDir: "/m" + strings.Repeat("x", i)})
	}
	if err := ValidateSpec(many, p); err == nil {
		t.Error("too many mounts")
	}
}

// The zero Policy is the real helper's: only /media.
func TestZeroPolicyIsMediaOnly(t *testing.T) {
	var p MountPolicy
	if err := p.CheckMount("/media/hdd/dreamhub-apps", "a", "/media/hdd/movie"); err != nil {
		t.Fatal(err)
	}
	if err := p.CheckMount("/media/hdd/dreamhub-apps", "a", "/etc"); err == nil {
		t.Fatal("/etc allowed by the zero policy")
	}
}

// The storage root is recorded on the container and used later for backup,
// restore and removal, so it is checked even for a container that mounts
// nothing from it.
func TestValidateSpec_StorageRoot(t *testing.T) {
	p := lexicalPolicy()
	for _, c := range []struct{ root, want string }{
		{"", ""},
		{testRoot, ""},
		{"/media/usb/dreamhub-apps", ""},
		{"/", "mount_dreamhub"}, // contains the protected data below /media
		{"/etc", "mount_system"},
		{"/etc/ssh", "mount_system"},
		{"/data/dreamhub-apps", "mount_system"},
		{"/var/lib/apps", "mount_system"},
		{"/tmp/apps", "mount_outside"},
		{"/media", "mount_dreamhub"}, // contains the protected data below it
		{"/media/hdd/dreamhub-data", "mount_dreamhub"},
		{"/media/../etc", "invalid"},
	} {
		spec := goodSpec()
		spec.Mounts = nil
		spec.Root = c.root
		if got := code(ValidateSpec(spec, p)); got != c.want {
			t.Errorf("root %q: %q, want %q", c.root, got, c.want)
		}
	}
	// "/" as a mount point inside the container.
	spec := goodSpec()
	spec.Mounts[0].ContainerDir = "/"
	if code(ValidateSpec(spec, p)) != "invalid" {
		t.Error("a mount at / in the container was allowed")
	}
}
