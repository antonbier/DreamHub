package appcatalog_test

import (
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
)

// boxPorts are the reference box's own ports (ARCHITECTURE.md): no app on
// the host network may need one of them.
var boxPorts = map[int]bool{80: true, 443: true, 8080: true, 8765: true, 8001: true, 4200: true, 8090: true, 139: true, 445: true}

func TestApps_HaveRequiredFields(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range appcatalog.Apps {
		if a.ID == "" || a.Name == "" || a.Image == "" || len(a.Arches) == 0 || a.ConfigPath == "" || a.MemoryMB <= 0 {
			t.Errorf("app %+v is missing a required field", a)
		}
		if seen[a.ID] {
			t.Errorf("duplicate app id %q", a.ID)
		}
		seen[a.ID] = true
		if boxPorts[a.ContainerPort] {
			t.Errorf("%s: web port %d is one of the box's own", a.ID, a.ContainerPort)
		}
		for _, p := range a.Ports {
			if boxPorts[p.Port] {
				t.Errorf("%s: port %d is one of the box's own", a.ID, p.Port)
			}
		}
		for _, s := range a.Services {
			// "data" and "secrets" name the app's other storage paths.
			if s.Name == "" || s.Name == "data" || !dockerd.ValidService(s.Name) || s.Image == "" || len(s.Arches) == 0 || s.ConfigPath == "" || s.MemoryMB <= 0 {
				t.Errorf("%s: service %+v is incomplete", a.ID, s)
			}
		}
	}
}

func TestApp_SupportsArch(t *testing.T) {
	piHole, _ := appcatalog.ByID("pihole")
	if !piHole.SupportsArch(dockerhub.Arch{Architecture: "arm", Variant: "v7"}) {
		t.Error("pihole should support armhf (arm/v7)")
	}
	// Nextcloud itself runs on armhf, its MariaDB doesn't: the app as a
	// whole doesn't.
	nc, ok := appcatalog.ByID("nextcloud")
	if !ok {
		t.Fatal("nextcloud missing")
	}
	if nc.SupportsArch(dockerhub.Arch{Architecture: "arm", Variant: "v7"}) {
		t.Error("nextcloud's database has no armhf image")
	}
	if !nc.SupportsArch(dockerhub.Arch{Architecture: "arm64"}) {
		t.Error("nextcloud should run on arm64 (the reference box)")
	}
}

func TestSecrets(t *testing.T) {
	nc, _ := appcatalog.ByID("nextcloud")
	names := nc.SecretNames()
	if len(names) != 2 || names[0] != "db" || names[1] != "dbroot" {
		t.Fatalf("got %v", names)
	}
	secrets := map[string]string{"db": "s1", "dbroot": "s2"}
	app := appcatalog.ExpandEnv(nc.Env, secrets)
	db := appcatalog.ExpandEnv(nc.Services[0].Env, secrets)
	if app["MYSQL_PASSWORD"] != "s1" || db["MARIADB_PASSWORD"] != "s1" || db["MARIADB_ROOT_PASSWORD"] != "s2" || app["MYSQL_HOST"] != "127.0.0.1" {
		t.Fatalf("app %v, db %v", app, db)
	}
	if nc.TotalMemoryMB() != 768 {
		t.Errorf("total memory %d", nc.TotalMemoryMB())
	}
}

func TestByID_Unknown(t *testing.T) {
	if _, ok := appcatalog.ByID("not-a-real-app"); ok {
		t.Fatal("got ok=true for an unknown id")
	}
}

func TestWithPort_ChangesCopyOnly(t *testing.T) {
	vw, _ := appcatalog.ByID("vaultwarden")
	moved := vw.WithPort(9000)
	if moved.ContainerPort != 9000 || moved.Env["ROCKET_PORT"] != "9000" {
		t.Fatalf("moved: %+v", moved)
	}
	if vw.ContainerPort != 8222 || vw.Env["ROCKET_PORT"] != "8222" {
		t.Fatal("the catalog entry itself changed")
	}
	nc, _ := appcatalog.ByID("nextcloud")
	moved = nc.WithPort(9001)
	if !strings.Contains(moved.Cmd[2], "Listen 9001") || !strings.Contains(moved.Cmd[2], "*:9001") || !strings.Contains(moved.Cmd[2], "^Listen 80$") {
		t.Fatalf("cmd: %s", moved.Cmd[2])
	}
	if !strings.Contains(nc.Cmd[2], "Listen 8088") {
		t.Fatal("the catalog entry's Cmd changed")
	}
	ha, _ := appcatalog.ByID("homeassistant")
	if ha.PortEditable() || ha.WithPort(1234).ContainerPort != 8123 {
		t.Fatal("an app with a fixed port moved")
	}
}
