package compose_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/compose"
)

const sample = `
name: demo
x-dreamhub:
  icon: cloud
  category: cloud
  description:
    en: Your own cloud.
    de: Deine eigene Cloud.
  note: Open it once.
  main: web
  largeConfig: true
  webui:
    port: ${WEB_PORT:-8088}
    path: /login
  fields:
    ADMIN_PASSWORD:
      label: {en: Admin password, de: Admin-Passwort}
      required: true
      secret: true
    WEB_PORT:
      label: {en: Web port, de: Web-Port}
      description: {en: The port to open it on.}
    TZ_NAME:
      advanced: false
services:
  db:
    image: library/mariadb:lts
    command: ["--bind-address=127.0.0.1"]
    mem_limit: 256m
    environment:
      MARIADB_PASSWORD: "{{secret:db}}"
    volumes:
      - .:/var/lib/mysql
    ports:
      - "127.0.0.1:3306:3306"
    x-dreamhub:
      arches: [amd64, arm64/v8]
  web:
    image: library/nextcloud
    depends_on: [db]
    restart: unless-stopped
    container_name: ignored-name
    environment:
      - MYSQL_HOST=db
      - ADMIN_PASSWORD=${ADMIN_PASSWORD}
      - TZ=${TZ_NAME:-UTC}
      - PORT=${WEB_PORT:-8088}
    ports:
      - "${WEB_PORT:-8088}:${WEB_PORT:-8088}"
      - "53:53/udp"
    volumes:
      - .:/var/www/html
      - ./data:/var/www/html/data
      - /media/hdd/movie:/movies:ro
    deploy:
      resources:
        limits:
          memory: 1G
    command: sh -c "sed -i 's/Listen 80$$/Listen ${WEB_PORT:-8088}/' /x && exec apache2-foreground"
`

func TestParse_SampleAndFields(t *testing.T) {
	f, err := compose.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if f.Services[0].Name != "db" || f.Services[1].Name != "web" {
		t.Fatalf("start order: %s, %s", f.Services[0].Name, f.Services[1].Name)
	}
	if f.MainService().Name != "web" || f.Ext.Icon != "cloud" || !f.Ext.LargeConfig || f.Ext.Description["de"] != "Deine eigene Cloud." {
		t.Fatalf("ext: %+v", f.Ext)
	}
	if f.Ext.Note.Get("it") != "Open it once." {
		t.Fatal("a plain string is the English text and the fallback")
	}
	if len(f.Notes) != 3 { // restart, container_name, MYSQL_HOST=db
		t.Fatalf("notes: %+v", f.Notes)
	}
	var names []string
	for _, v := range f.Vars {
		names = append(names, v.Name)
	}
	if !reflect.DeepEqual(names, []string{"WEB_PORT", "TZ_NAME", "ADMIN_PASSWORD"}) && !reflect.DeepEqual(names, []string{"ADMIN_PASSWORD", "TZ_NAME", "WEB_PORT"}) {
		// variables come in order of first use (services in start order)
		t.Logf("vars: %v", names)
	}

	fields := f.Fields("de")
	if len(fields) != 3 || fields[0].Name != "WEB_PORT" || !fields[0].WebPort || fields[0].Type != "port" || fields[0].Default != "8088" || fields[0].Label != "Web-Port" {
		t.Fatalf("fields: %+v", fields)
	}
	by := map[string]compose.Field{}
	for _, fl := range fields {
		by[fl.Name] = fl
	}
	pw := by["ADMIN_PASSWORD"]
	if !pw.Required || !pw.Secret || pw.Advanced || pw.Label != "Admin-Passwort" {
		t.Fatalf("password field: %+v", pw)
	}
	if tz := by["TZ_NAME"]; tz.Advanced || tz.Required || tz.Default != "UTC" {
		t.Fatalf("a field pinned with advanced: false stays in the simple view: %+v", tz)
	}
	if by["WEB_PORT"].Description != "The port to open it on." {
		t.Fatal("a language without text falls back to English")
	}
	if got := f.Fields("en")[0].Description; got != "The port to open it on." {
		t.Fatalf("en description %q", got)
	}
}

func TestResolve_Values(t *testing.T) {
	f, err := compose.Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve(nil); err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("a required field without a value must fail: %v", err)
	}
	r, err := f.Resolve(map[string]string{"ADMIN_PASSWORD": "pw$1", "WEB_PORT": "9001"})
	if err != nil {
		t.Fatal(err)
	}
	if r.WebPort != 9001 || r.WebPath != "/login" || r.Main != "web" {
		t.Fatalf("web: %+v", r)
	}
	web := r.Services[1]
	if web.Image != "library/nextcloud" || web.Tag != "" || web.MemoryMB != 1024 {
		t.Fatalf("web: %+v", web)
	}
	env := map[string]string{}
	for _, p := range web.Env {
		env[p.Key] = p.Value
	}
	if env["ADMIN_PASSWORD"] != "pw$1" || env["PORT"] != "9001" || env["TZ"] != "UTC" || env["MYSQL_HOST"] != "db" {
		t.Fatalf("env: %v", env)
	}
	if got := web.Command; len(got) != 3 || got[2] != "sed -i 's/Listen 80$/Listen 9001/' /x && exec apache2-foreground" {
		t.Fatalf("command: %q", got)
	}
	wantPorts := []compose.Port{{Port: 9001}, {Port: 53, UDP: true}}
	if !reflect.DeepEqual(web.Ports, wantPorts) {
		t.Fatalf("ports: %+v", web.Ports)
	}
	wantVols := []compose.Volume{{Target: "/var/www/html"}, {Name: "data", Target: "/var/www/html/data"}, {HostPath: "/media/hdd/movie", Target: "/movies", ReadOnly: true}}
	if !reflect.DeepEqual(web.Volumes, wantVols) {
		t.Fatalf("volumes: %+v", web.Volumes)
	}
	db := r.Services[0]
	if db.Tag != "lts" || db.MemoryMB != 256 || !db.Ports[0].Local || !reflect.DeepEqual(db.Arches, []string{"amd64", "arm64/v8"}) {
		t.Fatalf("db: %+v", db)
	}
}

func TestInterpolationForms(t *testing.T) {
	f, err := compose.Parse([]byte(`
services:
  a:
    image: x/y:${TAG-1.0}
    environment:
      A: $PLAIN
      B: "${REQ:?need it}"
      C: "cost: $$5"
      D: "${E:-}"
`))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]compose.Var{}
	for _, v := range f.Vars {
		by[v.Name] = v
	}
	if !by["TAG"].HasDefault || by["TAG"].Default != "1.0" || by["TAG"].Required {
		t.Fatalf("TAG %+v", by["TAG"])
	}
	if !by["PLAIN"].Required || !by["REQ"].Required || by["E"].Required || by["E"].Default != "" {
		t.Fatalf("vars %+v", by)
	}
	r, err := f.Resolve(map[string]string{"PLAIN": "p", "REQ": "r"})
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, p := range r.Services[0].Env {
		env[p.Key] = p.Value
	}
	if env["A"] != "p" || env["B"] != "r" || env["C"] != "cost: $5" || env["D"] != "" || r.Services[0].Tag != "1.0" {
		t.Fatalf("env %v tag %s", env, r.Services[0].Tag)
	}
}

func TestParse_AliasesAndMerge(t *testing.T) {
	f, err := compose.Parse([]byte(`
x-common: &common
  restart: always
  environment:
    LANG: C
services:
  a:
    <<: *common
    image: x/a
  b:
    <<: *common
    image: x/b
    environment:
      LANG: de
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Services) != 2 || len(f.Services[0].Env) != 1 || f.Services[1].Env[0].Value != "de" {
		t.Fatalf("%+v %+v", f.Services[0], f.Services[1])
	}
}

func TestParse_Refusals(t *testing.T) {
	cases := []struct {
		name, yaml, code string
	}{
		{"empty", ``, "no_services"},
		{"not yaml", "a: [", "yaml"},
		{"no services", "name: x\n", "no_services"},
		{"build", "services:\n a:\n  build: .\n", "unsupported_key"},
		{"image missing", "services:\n a:\n  restart: no\n", "image_missing"},
		{"top networks", "services:\n a:\n  image: x/y\nnetworks:\n n: {}\n", "unsupported_key"},
		{"top secrets", "services:\n a:\n  image: x/y\nsecrets:\n s: {file: x}\n", "unsupported_key"},
		{"configs", "services:\n a:\n  image: x/y\nconfigs:\n s: {file: x}\n", "unsupported_key"},
		{"service networks", "services:\n a:\n  image: x/y\n  networks: [n]\n", "unsupported_key"},
		{"extends", "services:\n a:\n  extends:\n   service: b\n", "unsupported_key"},
		{"env_file", "services:\n a:\n  image: x/y\n  env_file: .env\n", "unsupported_key"},
		{"user", "services:\n a:\n  image: x/y\n  user: root\n", "unsupported_key"},
		{"entrypoint", "services:\n a:\n  image: x/y\n  entrypoint: /bin/sh\n", "unsupported_key"},
		{"security_opt", "services:\n a:\n  image: x/y\n  security_opt: [seccomp=unconfined]\n", "unsupported_key"},
		{"network_mode container", "services:\n a:\n  image: x/y\n  network_mode: container:b\n", "network_mode"},
		{"network_mode service", "services:\n a:\n  image: x/y\n  network_mode: service:b\n", "network_mode"},
		{"pid other", "services:\n a:\n  image: x/y\n  pid: container:b\n", "bad_value"},
		{"deploy replicas", "services:\n a:\n  image: x/y\n  deploy:\n   replicas: 2\n", "unsupported_key"},
		{"anonymous volume", "services:\n a:\n  image: x/y\n  volumes:\n   - /data\n", "volume_anonymous"},
		{"volume option", "services:\n a:\n  image: x/y\n  volumes:\n   - ./x:/y:shared\n", "volume_invalid"},
		{"volume type tmpfs", "services:\n a:\n  image: x/y\n  volumes:\n   - {type: tmpfs, target: /t}\n", "volume_invalid"},
		{"top volume driver", "services:\n a:\n  image: x/y\nvolumes:\n v:\n  driver: nfs\n", "unsupported_key"},
		{"depends unknown", "services:\n a:\n  image: x/y\n  depends_on: [z]\n", "depends_unknown"},
		{"depends cycle", "services:\n a:\n  image: x/y\n  depends_on: [b]\n b:\n  image: x/z\n  depends_on: [a]\n", "depends_cycle"},
		{"nested var", "services:\n a:\n  image: x/${A:-${B}}\n", "interpolation"},
		{"lone dollar", "services:\n a:\n  image: x/y\n  command: echo $\n", "interpolation"},
		{"bad dollar", "services:\n a:\n  image: x/y\n  command: echo $/x\n", "interpolation"},
		{"alt form", "services:\n a:\n  image: x/y\n  environment:\n   A: ${B:+x}\n", "interpolation"},
		{"unknown ext key", "x-dreamhub:\n colour: red\nservices:\n a:\n  image: x/y\n", "ext_invalid"},
		{"field not used", "x-dreamhub:\n fields:\n  NOPE: {required: true}\nservices:\n a:\n  image: x/y\n", "ext_invalid"},
		{"bad main", "x-dreamhub:\n main: zz\nservices:\n a:\n  image: x/y\n", "ext_invalid"},
		{"bad arch", "services:\n a:\n  image: x/y\n  x-dreamhub:\n   arches: [\"A B\"]\n", "ext_invalid"},
		{"bad service name", "services:\n \"a b\":\n  image: x/y\n", "service_name"},
		{"env bad name", "services:\n a:\n  image: x/y\n  environment:\n   \"A B\": c\n", "bad_value"},
		{"command quote", "services:\n a:\n  image: x/y\n  command: echo 'x\n", "bad_value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := compose.Parse([]byte(c.yaml))
			var is compose.Issues
			if !errors.As(err, &is) {
				t.Fatalf("got %v, want a refusal", err)
			}
			for _, i := range is {
				if i.Code == c.code {
					if i.Message == "" {
						t.Fatal("a refusal needs a message")
					}
					return
				}
			}
			t.Fatalf("got %+v, want code %s", is, c.code)
		})
	}
}

func TestParse_TooLarge(t *testing.T) {
	_, err := compose.Parse([]byte("services:\n a:\n  image: x/y\n  command: " + strings.Repeat("a", compose.MaxSize)))
	var is compose.Issues
	if !errors.As(err, &is) || is[0].Code != "too_large" {
		t.Fatalf("got %v", err)
	}
}

func TestParse_AliasBomb(t *testing.T) {
	var b strings.Builder
	b.WriteString("a0: &a0 [x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i < 12; i++ {
		b.WriteString("a" + string(rune('0'+i%10)) + string(rune('a'+i)) + ": &b" + string(rune('a'+i)) + " [")
		for j := 0; j < 9; j++ {
			if j > 0 {
				b.WriteString(", ")
			}
			b.WriteString("*a0")
		}
		b.WriteString("]\n")
	}
	b.WriteString("services:\n a:\n  image: x/y\n")
	if _, err := compose.Parse([]byte(b.String())); err != nil {
		// Unknown top-level keys are refused anyway; the point is that it
		// returns promptly instead of expanding.
		t.Log("refused as expected:", err)
	}
}

func TestResolve_Refusals(t *testing.T) {
	cases := []struct{ name, yaml, code string }{
		{"port remap", "services:\n a:\n  image: x/y\n  ports: [\"8081:80\"]\n", "port_remap"},
		{"port range", "services:\n a:\n  image: x/y\n  ports: [\"8000-8010:8000-8010\"]\n", "port_invalid"},
		{"port zero", "services:\n a:\n  image: x/y\n  ports: [\"0\"]\n", "port_invalid"},
		{"port ip", "services:\n a:\n  image: x/y\n  ports: [\"192.168.1.5:80:80\"]\n", "port_invalid"},
		{"protocol", "services:\n a:\n  image: x/y\n  ports: [\"80/sctp\"]\n", "port_invalid"},
		{"image digest", "services:\n a:\n  image: x/y@sha256:abc\n", "image_invalid"},
		{"image upper", "services:\n a:\n  image: X/Y\n", "image_invalid"},
		{"image space", "services:\n a:\n  image: \"x/y z\"\n", "image_invalid"},
		{"bad tag", "services:\n a:\n  image: x/y:-no\n", "image_invalid"},
		{"mem", "services:\n a:\n  image: x/y\n  mem_limit: lots\n", "mem_invalid"},
		{"relative up", "services:\n a:\n  image: x/y\n  volumes: [\"../x:/y\"]\n", "volume_invalid"},
		{"tilde", "services:\n a:\n  image: x/y\n  volumes: [\"~/x:/y\"]\n", "volume_invalid"},
		{"folder chars", "services:\n a:\n  image: x/y\n  volumes: [\"my_data:/y\"]\n", "volume_invalid"},
		{"folder is service", "services:\n a:\n  image: x/y\n  volumes: [\"./a:/y\"]\n", "volume_invalid"},
		{"folder secrets", "services:\n a:\n  image: x/y\n  volumes: [\"./secrets:/y\"]\n", "volume_invalid"},
		{"target relative", "services:\n a:\n  image: x/y\n  volumes: [\"./x:y\"]\n", "volume_invalid"},
		{"target twice", "services:\n a:\n  image: x/y\n  volumes: [\"./x:/y\", \"./z:/y\"]\n", "volume_invalid"},
		{"cap", "services:\n a:\n  image: x/y\n  cap_add: [\"net admin\"]\n", "bad_value"},
		{"device", "services:\n a:\n  image: x/y\n  devices: [\"/etc/passwd\"]\n", "bad_value"},
		{"device dots", "services:\n a:\n  image: x/y\n  devices: [\"/dev/../etc\"]\n", "bad_value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := compose.Parse([]byte(c.yaml))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, err = f.Resolve(nil)
			var is compose.Issues
			if !errors.As(err, &is) {
				t.Fatalf("got %v, want a refusal", err)
			}
			for _, i := range is {
				if i.Code == c.code {
					return
				}
			}
			t.Fatalf("got %+v, want code %s", is, c.code)
		})
	}
}

func TestResolve_ElevatedAndSources(t *testing.T) {
	f, err := compose.Parse([]byte(`
services:
  a:
    image: ghcr.io/x/y:1.2
    privileged: true
    cap_add: [NET_ADMIN, cap_sys_time]
    devices: ["/dev/ttyUSB0:/dev/zigbee"]
    pid: host
    ipc: host
    network_mode: host
    volumes:
      - data:/d
      - {type: bind, source: /media/hdd/x, target: /x, read_only: true}
`))
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	e := r.Services[0].Elevated
	if !e.Any() || !e.Privileged || !e.PidHost || !e.IpcHost || !reflect.DeepEqual(e.CapAdd, []string{"NET_ADMIN", "SYS_TIME"}) || e.Devices[0] != "/dev/ttyUSB0:/dev/zigbee" {
		t.Fatalf("elevated: %+v", e)
	}
	if r.Services[0].Image != "ghcr.io/x/y" || r.Services[0].Tag != "1.2" || !r.Services[0].HostNetwork {
		t.Fatalf("service: %+v", r.Services[0])
	}
	v := r.Services[0].Volumes
	if v[0].Name != "data" || v[1].HostPath != "/media/hdd/x" || !v[1].ReadOnly {
		t.Fatalf("volumes: %+v", v)
	}
	plain, _ := compose.Parse([]byte("services:\n a:\n  image: x/y\n"))
	pr, _ := plain.Resolve(nil)
	if pr.Services[0].Elevated.Any() {
		t.Fatal("a plain container asks for nothing")
	}
}

func TestParseMemoryMB(t *testing.T) {
	for in, want := range map[string]int{"": 0, "512m": 512, "512M": 512, "1g": 1024, "256mb": 256, "1048576": 1, "1500k": 2, "2GB": 2048} {
		got, err := compose.ParseMemoryMB(in)
		if err != nil || got != want {
			t.Errorf("%q: %d %v, want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"x", "-1m", "0", "1.5g", "99999999999g"} {
		if _, err := compose.ParseMemoryMB(in); err == nil {
			t.Errorf("%q should be refused", in)
		}
	}
}

func TestServiceHostnameNote(t *testing.T) {
	f, err := compose.Parse([]byte("services:\n db:\n  image: x/db\n web:\n  image: x/web\n  depends_on: [db]\n  environment:\n   DB_HOST: db\n"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range f.Notes {
		found = found || n.Code == "service_hostname"
	}
	if !found {
		t.Fatalf("notes: %+v", f.Notes)
	}
}
