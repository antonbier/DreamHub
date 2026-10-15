package unraid_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/compose"
	"github.com/antonbier/DreamHub/backend/internal/unraid"
)

// A made-up template in the format Unraid documents (not a copy of any
// author's template).
const sample = `<?xml version="1.0"?>
<Container version="2">
  <Name>Example Notes</Name>
  <Repository>example/notes:1.4</Repository>
  <Registry>https://hub.docker.com/r/example/notes</Registry>
  <Network>bridge</Network>
  <Privileged>false</Privileged>
  <Overview>A small notes server.[br]With [b]markdown[/b] support.</Overview>
  <Icon>https://example.invalid/icon.png</Icon>
  <WebUI>http://[IP]:[PORT:8090]/app</WebUI>
  <ExtraParams>--cap-add=NET_ADMIN --memory=2g</ExtraParams>
  <Config Name="Web port" Target="80" Default="8090" Mode="tcp" Description="Where to reach it" Type="Port" Required="true">8090</Config>
  <Config Name="Sync" Target="9000" Default="9000" Mode="udp" Type="Port">9000</Config>
  <Config Name="Appdata" Target="/config" Default="/mnt/user/appdata/notes" Mode="rw" Description="Settings" Type="Path">/mnt/user/appdata/notes</Config>
  <Config Name="Library" Target="/library" Default="/mnt/user/library" Mode="ro" Type="Path"/>
  <Config Name="Timezone" Target="TZ" Default="Europe/Berlin" Mode="" Type="Variable">Europe/Rome</Config>
  <Config Name="Admin password" Target="ADMIN_PASSWORD" Default="" Mode="" Type="Variable" Required="true" Mask="true"></Config>
  <Config Name="Cost" Target="PRICE" Default="$5" Mode="" Type="Variable">$5</Config>
  <Config Name="GPU" Target="/dev/dri" Default="/dev/dri" Mode="" Type="Device">/dev/dri</Config>
  <Config Name="Traefik" Target="traefik.enable" Default="true" Mode="" Type="Label">true</Config>
</Container>`

func TestConvert(t *testing.T) {
	text, notes, err := unraid.Convert([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	f, err := compose.Parse([]byte(text))
	if err != nil {
		t.Fatalf("the result must be a valid compose file: %v\n%s", err, text)
	}
	if f.Ext.Name != "Example Notes" || f.Ext.Description["en"] != "A small notes server. With markdown support." || f.Ext.WebPath != "/app" || f.Ext.WebPort != "8090" {
		t.Fatalf("ext: %+v", f.Ext)
	}
	r, err := f.Resolve(map[string]string{"ADMIN_PASSWORD": "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	s := r.Services[0]
	if s.Image != "example/notes" || s.Tag != "1.4" {
		t.Fatalf("image: %+v", s)
	}
	if len(s.Ports) != 2 || s.Ports[0].Port != 80 || s.Ports[1].Port != 9000 || !s.Ports[1].UDP {
		t.Fatalf("ports (the container port is the app's port): %+v", s.Ports)
	}
	if len(s.Volumes) != 2 || s.Volumes[0].Name != "appdata" || s.Volumes[0].Target != "/config" || s.Volumes[1].Name != "library" || !s.Volumes[1].ReadOnly {
		t.Fatalf("volumes (the template's host paths become app folders): %+v", s.Volumes)
	}
	env := map[string]string{}
	for _, p := range s.Env {
		env[p.Key] = p.Value
	}
	if env["TZ"] != "Europe/Rome" || env["ADMIN_PASSWORD"] != "s3cret" || env["PRICE"] != "$5" {
		t.Fatalf("env: %v", env)
	}
	if s.Elevated.Any() {
		t.Fatal("nothing elevated in this template")
	}

	fields := map[string]compose.Field{}
	for _, fl := range f.Fields("en") {
		fields[fl.Name] = fl
	}
	if pw := fields["ADMIN_PASSWORD"]; !pw.Required || !pw.Secret || pw.Label != "Admin password" {
		t.Fatalf("password field: %+v", pw)
	}
	if p := fields["PATH_LIBRARY"]; p.Type != "path" || p.Default != "./library" || p.Label != "Library" {
		t.Fatalf("path field: %+v", p)
	}
	if tz := fields["TZ"]; tz.Default != "Europe/Rome" || tz.Required {
		t.Fatalf("variable field: %+v", tz)
	}

	codes := map[string]bool{}
	for _, n := range notes {
		codes[n.Code] = true
	}
	for _, want := range []string{"unraid_icon", "unraid_extraparams", "unraid_ignored", "unraid_port_remap", "unraid_paths"} {
		if !codes[want] {
			t.Errorf("no %s note: %+v", want, notes)
		}
	}
	for _, n := range notes {
		if n.Code == "unraid_ignored" && (!strings.Contains(n.Detail, "/dev/dri") || !strings.Contains(n.Detail, "traefik.enable")) {
			t.Errorf("the ignored note should name the device and the label: %+v", n)
		}
	}
	if strings.Contains(text, "NET_ADMIN") || strings.Contains(text, "--memory") || strings.Contains(text, "/dev/dri") || strings.Contains(text, "/mnt/user") {
		t.Fatalf("extra params, devices and Unraid paths must not reach the compose file:\n%s", text)
	}
}

func TestConvert_PrivilegedIsKeptSoItNeedsConfirmation(t *testing.T) {
	text, notes, err := unraid.Convert([]byte(`<Container version="2"><Name>x</Name><Repository>a/b</Repository><Privileged>true</Privileged></Container>`))
	if err != nil {
		t.Fatal(err)
	}
	f, err := compose.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := f.Resolve(nil)
	if !r.Services[0].Elevated.Privileged {
		t.Fatal("privileged was dropped")
	}
	if len(notes) == 0 || notes[0].Code != "unraid_privileged" {
		t.Fatalf("notes: %+v", notes)
	}
}

func TestConvert_Refusals(t *testing.T) {
	for name, in := range map[string]string{
		"not xml":       `services: {}`,
		"wrong root":    `<Other><Repository>a/b</Repository></Other>`,
		"no repository": `<Container version="2"><Name>x</Name></Container>`,
		"digest":        `<Container version="2"><Repository>a/b@sha256:abc</Repository></Container>`,
		"bad image":     `<Container version="2"><Repository>A B</Repository></Container>`,
		"truncated":     `<Container version="2"><Repository>a/b`,
		"too large":     `<Container version="2"><Overview>` + strings.Repeat("x", unraid.MaxSize) + `</Overview></Container>`,
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := unraid.Convert([]byte(in))
			var is compose.Issues
			if !errors.As(err, &is) || len(is) == 0 || is[0].Message == "" {
				t.Fatalf("got %v, want a refusal", err)
			}
		})
	}
}

// XML entities and DOCTYPE tricks don't reach the network or the disk: Go's
// decoder resolves nothing external.
func TestConvert_NoExternalEntities(t *testing.T) {
	_, _, err := unraid.Convert([]byte(`<?xml version="1.0"?><!DOCTYPE c [<!ENTITY x SYSTEM "file:///etc/passwd">]><Container version="2"><Name>&x;</Name><Repository>a/b</Repository></Container>`))
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "root:") {
		t.Fatal("an external entity was read")
	}
}
