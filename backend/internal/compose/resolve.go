package compose

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Elevated lists what a service asks for beyond a plain container. Every
// part needs the person's explicit extra confirmation (internal/hubs), and
// internal/dockerd refuses to create such a container without it.
type Elevated struct {
	Privileged bool     `json:"privileged,omitempty"`
	CapAdd     []string `json:"capAdd,omitempty"`
	Devices    []string `json:"devices,omitempty"`
	PidHost    bool     `json:"pidHost,omitempty"`
	IpcHost    bool     `json:"ipcHost,omitempty"`
}

// Any reports whether anything is asked for.
func (e Elevated) Any() bool {
	return e.Privileged || len(e.CapAdd) > 0 || len(e.Devices) > 0 || e.PidHost || e.IpcHost
}

// Port is one port a container binds on the box (apps share the box's
// network, so the host and container port are the same number).
type Port struct {
	Port int  `json:"port"`
	UDP  bool `json:"udp,omitempty"`
	// Local: bound on 127.0.0.1 only.
	Local bool `json:"local,omitempty"`
}

// Volume is one mount. Exactly one of the three kinds:
//   - Name == "" and HostPath == "": the service's own folder
//     (<storage>/<app> for the main service, <storage>/<app>.<service>
//     for a helper) — written "." in the file;
//   - Name != "": a folder shared by the app (<storage>/<app>.<name>) —
//     "./name" or a named volume in the file;
//   - HostPath != "": an absolute path on the box; whether it is allowed
//     is decided by internal/dockerd's mount rules, not here.
type Volume struct {
	Name     string `json:"name,omitempty"`
	HostPath string `json:"hostPath,omitempty"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

// Own reports whether v is the service's own folder.
func (v Volume) Own() bool { return v.Name == "" && v.HostPath == "" }

// RService is a service with every variable substituted and every value
// typed.
type RService struct {
	Name      string
	Image     string
	Tag       string
	Command   []string
	Env       []Pair
	Ports     []Port
	Volumes   []Volume
	MemoryMB  int // 0: not set in the file
	DependsOn []string
	Elevated  Elevated
	// HostNetwork was written explicitly; apps use the box's network
	// either way.
	HostNetwork bool
	Arches      []string
}

// Resolved is a compose file with its form values applied.
type Resolved struct {
	Name     string
	Services []RService // start order
	Main     string     // name of the main service
	WebPort  int        // 0: no web UI
	WebPath  string
	Notes    []Issue
}

var (
	imageNameRe = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)
	hostPartRe  = regexp.MustCompile(`^[a-zA-Z0-9]+(?:[.-][a-zA-Z0-9]+)*(?::[0-9]{1,5})?$`)
	tagRe       = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	capRe       = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,31}$`)
	folderRe    = regexp.MustCompile(`^[a-z0-9]{1,20}$`)
	memoryRe    = regexp.MustCompile(`^([0-9]+)([bBkKmMgG]?)[bB]?$`)
)

// SplitImage splits "name[:tag]" and validates both. Digest pins
// (name@sha256:…) are refused: DreamHub follows tags (its update check
// compares a tag's digest), so a pin could never be updated.
func SplitImage(ref string) (image, tag string, err error) {
	if ref == "" {
		return "", "", fmt.Errorf("the image name is empty")
	}
	if strings.ContainsAny(ref, "@ \t\n\\") {
		return "", "", fmt.Errorf("the image %q is not a plain name:tag (digest pins are not supported)", ref)
	}
	name := ref
	if i := strings.LastIndex(ref, ":"); i >= 0 && !strings.Contains(ref[i:], "/") {
		name, tag = ref[:i], ref[i+1:]
		if !tagRe.MatchString(tag) {
			return "", "", fmt.Errorf("the tag %q is not valid", tag)
		}
	}
	parts := strings.Split(name, "/")
	for i, part := range parts {
		ok := imageNameRe.MatchString(part)
		if i == 0 && len(parts) > 1 {
			ok = ok || hostPartRe.MatchString(part)
		}
		if !ok || len(name) > 255 {
			return "", "", fmt.Errorf("the image name %q is not valid (lowercase letters, digits and . _ - only)", name)
		}
	}
	return name, tag, nil
}

// ParseMemoryMB reads "512m", "1g", "256mb" or plain bytes into whole MB
// (rounded up); "" is 0.
func ParseMemoryMB(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	m := memoryRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("%q is not a memory size like 512m or 1g", s)
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || n <= 0 || n > 1<<30 {
		return 0, fmt.Errorf("%q is not a usable memory size", s)
	}
	switch strings.ToLower(m[2]) {
	case "k":
		n <<= 10
	case "m":
		n <<= 20
	case "g":
		n <<= 30
	}
	mb := (n + (1 << 20) - 1) >> 20
	if mb > 1<<20 {
		return 0, fmt.Errorf("%q is more memory than any box has", s)
	}
	return int(mb), nil
}

type resolver struct {
	f      *File
	values map[string]string
	issues Issues
	svc    string
}

func (r *resolver) bad(code, key, detail, format string, args ...any) {
	r.issues = append(r.issues, issue(code, r.svc, key, detail, format, args...))
}

func (r *resolver) sub(s, key string) string {
	out, err := substitute(s, r.values, r.f.defs)
	if err != nil {
		r.bad("missing_value", key, "", "%v", err)
		return ""
	}
	return out
}

func (r *resolver) subAll(in []string, key string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = r.sub(s, key)
	}
	return out
}

func (r *resolver) port(s, key string) int {
	s = strings.TrimSpace(s)
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		r.bad("port_invalid", key, s, "%q is not a port number (1-65535; ranges are not supported)", s)
		return 0
	}
	return n
}

// Resolve applies values (a map of variable name → value; a missing or
// empty one falls back to the file's default) and returns the typed
// result, or every reason it can't be.
func (f *File) Resolve(values map[string]string) (*Resolved, error) {
	r := &resolver{f: f, values: values}
	// A required variable without a value is the first thing to report.
	for _, v := range f.Vars {
		if v.Required && values[v.Name] == "" {
			r.issues = append(r.issues, issue("missing_value", "", v.Name, "", "a value for %s is required", v.Name))
		}
	}
	if len(r.issues) > 0 {
		return nil, r.issues
	}
	res := &Resolved{Name: f.Name, Main: f.MainService().Name, WebPath: f.Ext.WebPath, Notes: f.Notes}
	if res.WebPath == "" {
		res.WebPath = "/"
	}
	if f.Ext.WebPort != "" {
		r.svc = ""
		// "" or 0 (an optional web port left empty) means no web UI.
		if w := strings.TrimSpace(r.sub(f.Ext.WebPort, "webui.port")); w != "" && w != "0" {
			res.WebPort = r.port(w, "webui.port")
		}
	}
	names := map[string]bool{}
	for _, s := range f.Services {
		names[s.Name] = true
	}
	for _, s := range f.Services {
		r.svc = s.Name
		res.Services = append(res.Services, r.service(s, names))
	}
	if len(r.issues) > 0 {
		return nil, r.issues
	}
	return res, nil
}

func (r *resolver) service(s *Service, services map[string]bool) RService {
	out := RService{Name: s.Name, DependsOn: s.DependsOn, HostNetwork: s.HostNetwork}
	img := r.sub(s.Image, "image")
	var err error
	if out.Image, out.Tag, err = SplitImage(img); err != nil {
		r.bad("image_invalid", "image", img, "service %s: %v", s.Name, err)
	}
	out.Command = r.subAll(s.Command, "command")
	for _, e := range s.Env {
		out.Env = append(out.Env, Pair{e.Key, r.sub(e.Value, "environment")})
	}
	seen := map[Port]bool{}
	for _, ps := range s.Ports {
		target := r.port(r.sub(ps.Target, "ports"), "ports")
		pub := target
		if ps.Published != "" {
			pub = r.port(r.sub(ps.Published, "ports"), "ports")
		}
		if target != 0 && pub != 0 && pub != target {
			r.bad("port_remap", "ports", fmt.Sprintf("%d:%d", pub, target),
				"service %s: port %d can't be mapped to %d; apps use the box's own network, so the app listens on %d itself (change its port with its own setting, e.g. an environment variable)",
				s.Name, target, pub, target)
		}
		p := Port{Port: target}
		switch strings.ToLower(r.sub(ps.Protocol, "ports")) {
		case "", "tcp":
		case "udp":
			p.UDP = true
		default:
			r.bad("port_invalid", "ports", ps.Protocol, "service %s: the protocol %q isn't tcp or udp", s.Name, ps.Protocol)
		}
		switch ip := r.sub(ps.HostIP, "ports"); ip {
		case "", "0.0.0.0":
		case "127.0.0.1", "localhost":
			p.Local = true
		default:
			r.bad("port_invalid", "ports", ip, "service %s: the address %q isn't supported (only 127.0.0.1)", s.Name, ip)
		}
		if p.Port != 0 && !seen[p] {
			seen[p] = true
			out.Ports = append(out.Ports, p)
		}
	}
	targets := map[string]bool{}
	for _, v := range s.Volumes {
		rv, ok := r.volume(s, v, services)
		if !ok {
			continue
		}
		if targets[rv.Target] {
			r.bad("volume_invalid", "volumes", rv.Target, "service %s: two volumes are mounted at %s", s.Name, rv.Target)
			continue
		}
		targets[rv.Target] = true
		out.Volumes = append(out.Volumes, rv)
	}
	if mb, err := ParseMemoryMB(r.sub(s.Memory, "mem_limit")); err != nil {
		r.bad("mem_invalid", "mem_limit", s.Memory, "service %s: %v", s.Name, err)
	} else {
		out.MemoryMB = mb
	}
	out.Elevated = Elevated{Privileged: s.Privileged, PidHost: s.PidHost, IpcHost: s.IpcHost}
	for _, c := range r.subAll(s.CapAdd, "cap_add") {
		c = strings.TrimPrefix(strings.ToUpper(c), "CAP_")
		if !capRe.MatchString(c) {
			r.bad("bad_value", "cap_add", c, "service %s: %q is not a capability name", s.Name, c)
			continue
		}
		out.Elevated.CapAdd = append(out.Elevated.CapAdd, c)
	}
	for _, d := range r.subAll(s.Devices, "devices") {
		if !validDevice(d) {
			r.bad("bad_value", "devices", d, "service %s: %q is not a device like /dev/ttyUSB0 or /dev/ttyUSB0:/dev/zigbee", s.Name, d)
			continue
		}
		out.Elevated.Devices = append(out.Elevated.Devices, d)
	}
	out.Arches = s.Arches
	return out
}

// validDevice checks the shape of "host[:container[:perms]]": an absolute,
// clean path below /dev on both sides. Which devices are allowed is
// internal/dockerd's decision.
func validDevice(d string) bool {
	parts := strings.Split(d, ":")
	if len(parts) < 1 || len(parts) > 3 {
		return false
	}
	for i, p := range parts {
		if i == 2 {
			if p == "" || strings.Trim(p, "rwm") != "" {
				return false
			}
			continue
		}
		if !strings.HasPrefix(p, "/dev/") || path.Clean(p) != p || strings.ContainsAny(p, "\x00\n\r,") {
			return false
		}
	}
	return true
}

// volume classifies a mount's source (see Volume).
func (r *resolver) volume(s *Service, v VolumeSpec, services map[string]bool) (Volume, bool) {
	src := r.sub(v.Source, "volumes")
	target := r.sub(v.Target, "volumes")
	if !strings.HasPrefix(target, "/") || path.Clean(target) != target || strings.ContainsAny(target, ":\x00\n\r,") || len(target) > 512 {
		r.bad("volume_invalid", "volumes", target, "service %s: the mount point %q must be a clean absolute path inside the container", s.Name, target)
		return Volume{}, false
	}
	out := Volume{Target: target, ReadOnly: v.ReadOnly}
	switch {
	case src == "." || src == "./":
	case strings.HasPrefix(src, "/"):
		out.HostPath = src
	case strings.HasPrefix(src, "./") || !strings.ContainsAny(src, "/~$"):
		name := strings.TrimPrefix(src, "./")
		if !folderRe.MatchString(name) {
			r.bad("volume_invalid", "volumes", src, "service %s: the folder name %q must be 1-20 lowercase letters and digits", s.Name, name)
			return Volume{}, false
		}
		if services[name] || name == "secrets" {
			r.bad("volume_invalid", "volumes", src, "service %s: the folder name %q is taken (a service or DreamHub's own)", s.Name, name)
			return Volume{}, false
		}
		out.Name = name
	default:
		r.bad("volume_invalid", "volumes", src, "service %s: the source %q is not understood (use . or ./name for the app's storage, or an absolute path)", s.Name, src)
		return Volume{}, false
	}
	return out, true
}
