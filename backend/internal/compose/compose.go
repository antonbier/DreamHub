// Package compose reads a docker-compose.yml into DreamHub's own app model
// (docs/APPSHUB_PLAN.md, App12). It is deliberately a small, strict subset
// of the Compose spec: what DreamHub's isolated Docker helper can create
// (internal/dockerd) is exactly what can be expressed here, and every key
// outside that subset is refused with a clear message instead of being
// silently ignored — a compose file that "worked" while quietly dropping
// its privileged or volume settings would be the dangerous kind of wrong.
//
// Supported per service: image, ports, volumes, environment, command,
// mem_limit / deploy.resources.limits.memory, restart, depends_on (as
// start order), network_mode (host or bridge), and — only ever allowed
// with an explicit extra confirmation, see Resolved.Elevated — privileged,
// cap_add, devices, pid: host, ipc: host. Refused: build, networks,
// secrets, configs, extends, env_file, user, entrypoint, security_opt,
// links, tmpfs, sysctls, and everything else not listed.
//
// Parsing is pure: no network, no file access, no Docker. The `x-dreamhub`
// extension carries DreamHub's own layer (icon, category, descriptions in
// de/en/it, web UI port and path, form fields); Parse also collects every
// ${VARIABLE:-default} as a form field (Vars).
package compose

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// MaxSize is the largest compose file Parse reads.
const MaxSize = 256 << 10

// maxNodes bounds how many YAML nodes Parse visits (anchors and aliases can
// otherwise multiply a small file into a huge one).
const maxNodes = 50000

// Text is a string in several languages ("de", "en", "it").
type Text map[string]string

// Get returns the text for lang, else English, else the first language by
// name — "" if there is none.
func (t Text) Get(lang string) string {
	if v := t[lang]; v != "" {
		return v
	}
	if v := t["en"]; v != "" {
		return v
	}
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if t[k] != "" {
			return t[k]
		}
	}
	return ""
}

// Pair is one environment variable, value possibly holding ${…}.
type Pair struct{ Key, Value string }

// PortSpec is one ports entry before interpolation.
type PortSpec struct {
	Published string // "" when only the container port was given
	Target    string
	Protocol  string // "", "tcp" or "udp"
	HostIP    string
}

// VolumeSpec is one volumes entry before interpolation.
type VolumeSpec struct {
	Source, Target string
	ReadOnly       bool
}

// Service is one compose service as written.
type Service struct {
	Name        string
	Image       string
	Command     []string
	Env         []Pair
	Ports       []PortSpec
	Volumes     []VolumeSpec
	Memory      string // as written, e.g. "512m"
	DependsOn   []string
	Privileged  bool
	CapAdd      []string
	Devices     []string
	PidHost     bool
	IpcHost     bool
	HostNetwork bool // network_mode: host written explicitly
	// Arches is x-dreamhub.arches on the service: platforms verified for
	// its image ("amd64", "arm64", "arm/v7", …).
	Arches []string
}

// FieldMeta is the x-dreamhub.fields entry for one variable.
type FieldMeta struct {
	Label       Text
	Description Text
	Required    bool
	Secret      bool
	// Advanced: nil means the default (advanced unless it is the web port
	// or required); false pins a field to the simple view.
	Advanced *bool
	// Type overrides what the form infers from where the variable is used:
	// "port", "path", "number" or "text".
	Type string
}

// Ext is the top-level x-dreamhub block.
type Ext struct {
	Name        string
	Icon        string
	Category    string
	Description Text
	Note        Text
	// WebPort is the web UI port as written (a number or ${VAR:-n}); ""
	// means the app has no web UI. WebPath is the path to open ("/").
	WebPort string
	WebPath string
	// Main names the main service (the one with the web UI); else the first
	// service nothing else depends on.
	Main string
	// LargeConfig: the app's own folder holds its code too, so it is not in
	// a backup unless picked (Nextcloud).
	LargeConfig bool
	Fields      map[string]FieldMeta
	// FieldOrder is the order the fields were written in.
	FieldOrder []string
}

// File is a parsed compose file.
type File struct {
	Name string
	// Services are in start order: what a service depends on comes first.
	Services []*Service
	Ext      Ext
	Vars     []Var
	// Notes are things that were understood but are worth knowing (a
	// cosmetic key that is ignored, a service name used as a host name).
	Notes []Issue
	defs  map[string]*Var
}

var (
	supportedService = map[string]bool{
		"image": true, "ports": true, "volumes": true, "environment": true, "command": true,
		"mem_limit": true, "deploy": true, "restart": true, "depends_on": true, "network_mode": true,
		"privileged": true, "cap_add": true, "devices": true, "pid": true, "ipc": true,
		"x-dreamhub": true,
	}
	// ignoredService keys change nothing DreamHub manages itself.
	ignoredService = map[string]bool{
		"container_name": true, "labels": true, "healthcheck": true, "hostname": true,
		"tty": true, "stdin_open": true, "logging": true, "pull_policy": true, "platform": true,
		"stop_grace_period": true, "init": true,
	}
	serviceNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	envKeyRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
)

type parser struct {
	f     *File
	vars  *vars
	issue Issues
	seen  int
	// svc is the service being read, for messages.
	svc string
}

func (p *parser) bad(code, key, detail, format string, args ...any) {
	p.issue = append(p.issue, issue(code, p.svc, key, detail, format, args...))
}

func (p *parser) note(code, key, detail, format string, args ...any) {
	p.f.Notes = append(p.f.Notes, issue(code, p.svc, key, detail, format, args...))
}

// deref follows aliases and counts the visit.
func (p *parser) deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
		p.seen++
	}
	p.seen++
	if p.seen > maxNodes {
		if p.seen == maxNodes+1 {
			p.bad("too_large", "", "", "the file expands to too many items (check its anchors and aliases)")
		}
		return nil
	}
	return n
}

// mapping is a YAML mapping with merge keys (<<) resolved, keys in order.
type mapping struct {
	keys []string
	vals map[string]*yaml.Node
}

func (p *parser) asMap(n *yaml.Node, key string) *mapping {
	n = p.deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		p.bad("bad_value", key, "", "%s must be a mapping", p.where(key))
		return nil
	}
	m := &mapping{vals: map[string]*yaml.Node{}}
	var merged []*mapping
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		if k.Kind != yaml.ScalarNode {
			p.bad("bad_value", key, "", "%s has a key that is not a plain word", p.where(key))
			return nil
		}
		if k.Value == "<<" {
			v := p.deref(n.Content[i+1])
			if v == nil {
				return nil
			}
			switch v.Kind {
			case yaml.MappingNode:
				if mm := p.asMap(v, key); mm != nil {
					merged = append(merged, mm)
				}
			case yaml.SequenceNode:
				for _, e := range v.Content {
					if mm := p.asMap(e, key); mm != nil {
						merged = append(merged, mm)
					}
				}
			}
			continue
		}
		if _, dup := m.vals[k.Value]; dup {
			p.bad("bad_value", k.Value, "", "%s is written twice", p.where(k.Value))
			continue
		}
		m.keys = append(m.keys, k.Value)
		m.vals[k.Value] = n.Content[i+1]
	}
	for _, mm := range merged { // explicit keys win over merged ones
		for _, k := range mm.keys {
			if _, ok := m.vals[k]; !ok {
				m.keys = append(m.keys, k)
				m.vals[k] = mm.vals[k]
			}
		}
	}
	return m
}

func (p *parser) where(key string) string {
	switch {
	case p.svc != "" && key != "":
		return "service " + p.svc + ": " + key
	case p.svc != "":
		return "service " + p.svc
	case key != "":
		return key
	}
	return "the file"
}

// scalar returns a scalar node's text; interpolation syntax is validated
// and the variables registered.
func (p *parser) scalar(n *yaml.Node, key string) (string, bool) {
	n = p.deref(n)
	if n == nil {
		return "", false
	}
	if n.Kind != yaml.ScalarNode {
		p.bad("bad_value", key, "", "%s must be a single value", p.where(key))
		return "", false
	}
	if n.Tag == "!!null" {
		return "", true
	}
	return n.Value, p.scan(n.Value, key)
}

func (p *parser) scan(s, key string) bool {
	segs, err := splitRefs(s)
	if err != nil {
		p.bad("interpolation", key, err.Error(), "%s: %v", p.where(key), err)
		return false
	}
	for _, sg := range segs {
		if sg.ref != nil {
			p.vars.add(sg.ref)
		}
	}
	return true
}

// list reads a sequence of scalars.
func (p *parser) list(n *yaml.Node, key string) []string {
	n = p.deref(n)
	if n == nil {
		return nil
	}
	if n.Kind != yaml.SequenceNode {
		p.bad("bad_value", key, "", "%s must be a list", p.where(key))
		return nil
	}
	var out []string
	for _, e := range n.Content {
		if s, ok := p.scalar(e, key); ok {
			out = append(out, s)
		}
	}
	return out
}

// Parse reads a compose file. A refused file returns Issues (every reason
// found, not only the first).
func Parse(data []byte) (*File, error) {
	if len(data) > MaxSize {
		return nil, Issues{issue("too_large", "", "", "", "the compose file is larger than %d KB", MaxSize>>10)}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, Issues{issue("yaml", "", "", err.Error(), "this is not valid YAML: %v", err)}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, Issues{issue("no_services", "", "", "", "the file is empty")}
	}
	f := &File{}
	p := &parser{f: f, vars: newVars()}
	root := p.asMap(doc.Content[0], "")
	if root == nil {
		return nil, p.result()
	}
	var servicesNode, extNode *yaml.Node
	for _, k := range root.keys {
		v := root.vals[k]
		switch {
		case k == "services":
			servicesNode = v
		case k == "name":
			f.Name, _ = p.scalar(v, "name")
		case k == "version":
			p.note("version_ignored", "version", "", "\"version\" is obsolete and ignored")
		case k == "volumes":
			p.topVolumes(v)
		case k == "x-dreamhub":
			extNode = v
		case strings.HasPrefix(k, "x-"):
			// another tool's extension: nothing to do
		case k == "networks" || k == "secrets" || k == "configs" || k == "include":
			p.bad("unsupported_key", k, "", "%q is not supported: apps run on the box's own network and DreamHub has no secrets or configs of its own", k)
		default:
			p.bad("unsupported_key", k, "", "%q is not supported", k)
		}
	}
	if servicesNode == nil {
		p.bad("no_services", "services", "", "the file has no services")
	} else {
		p.services(servicesNode)
	}
	if extNode != nil {
		p.ext(extNode)
	}
	if len(p.issue) > 0 {
		return nil, p.result()
	}
	p.order()
	if len(p.issue) == 0 {
		p.checkExt()
		p.hints()
	}
	if len(p.issue) > 0 {
		return nil, p.result()
	}
	f.Vars = p.vars.list()
	f.defs = p.vars.by
	return f, nil
}

func (p *parser) result() error {
	if len(p.issue) == 0 {
		return Issues{issue("yaml", "", "", "", "the file could not be read")}
	}
	return p.issue
}

// topVolumes accepts only plain named volumes (null or {}).
func (p *parser) topVolumes(n *yaml.Node) {
	n = p.deref(n)
	if n == nil || n.Tag == "!!null" {
		return
	}
	m := p.asMap(n, "volumes")
	if m == nil {
		return
	}
	for _, k := range m.keys {
		v := p.deref(m.vals[k])
		if v == nil || v.Tag == "!!null" || (v.Kind == yaml.MappingNode && len(v.Content) == 0) {
			continue
		}
		p.bad("unsupported_key", "volumes."+k, "", "the volume %q has options; DreamHub keeps named volumes as folders in the app's storage and supports no driver options", k)
	}
}

func (p *parser) services(n *yaml.Node) {
	m := p.asMap(n, "services")
	if m == nil {
		return
	}
	if len(m.keys) == 0 {
		p.bad("no_services", "services", "", "the file has no services")
		return
	}
	for _, name := range m.keys {
		p.svc = name
		if !serviceNameRe.MatchString(name) {
			p.bad("service_name", "", name, "the service name %q uses characters that are not allowed", name)
			continue
		}
		if s := p.service(name, m.vals[name]); s != nil {
			p.f.Services = append(p.f.Services, s)
		}
	}
	p.svc = ""
}

func (p *parser) service(name string, n *yaml.Node) *Service {
	m := p.asMap(n, "")
	if m == nil {
		return nil
	}
	s := &Service{Name: name}
	for _, k := range m.keys {
		v := m.vals[k]
		switch {
		case ignoredService[k]:
			p.note("key_ignored", k, "", "service %s: %q is ignored (DreamHub manages that itself)", name, k)
		case !supportedService[k]:
			p.bad("unsupported_key", k, "", "service %s: %q is not supported", name, k)
		case k == "image":
			s.Image, _ = p.scalar(v, k)
		case k == "command":
			s.Command = p.command(v)
		case k == "environment":
			s.Env = p.environment(v)
		case k == "ports":
			s.Ports = p.ports(v)
		case k == "volumes":
			s.Volumes = p.volumes(v)
		case k == "mem_limit":
			s.Memory, _ = p.scalar(v, k)
		case k == "deploy":
			s.Memory = p.deploy(v, s.Memory)
		case k == "restart":
			if _, ok := p.scalar(v, k); ok {
				p.note("restart_managed", k, "", "service %s: restart is managed by DreamHub (a crashing app is restarted a few times, then reported)", name)
			}
		case k == "depends_on":
			s.DependsOn = p.dependsOn(v)
		case k == "network_mode":
			mode, _ := p.scalar(v, k)
			switch mode {
			case "host":
				s.HostNetwork = true
			case "bridge", "default", "":
			default:
				p.bad("network_mode", k, mode, "service %s: network_mode %q is not supported (apps use the box's network)", name, mode)
			}
		case k == "privileged":
			s.Privileged = p.boolean(v, k)
		case k == "cap_add":
			s.CapAdd = p.list(v, k)
		case k == "devices":
			s.Devices = p.list(v, k)
		case k == "pid":
			s.PidHost = p.hostOnly(v, k)
		case k == "ipc":
			s.IpcHost = p.hostOnly(v, k)
		case k == "x-dreamhub":
			p.serviceExt(s, v)
		}
	}
	if s.Image == "" {
		if _, hasBuild := m.vals["build"]; !hasBuild {
			p.bad("image_missing", "image", "", "service %s has no image", name)
		}
	}
	return s
}

func (p *parser) boolean(n *yaml.Node, key string) bool {
	n = p.deref(n)
	if n == nil {
		return false
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!bool" {
		return n.Value == "true"
	}
	p.bad("bad_value", key, "", "%s must be true or false", p.where(key))
	return false
}

func (p *parser) hostOnly(n *yaml.Node, key string) bool {
	v, ok := p.scalar(n, key)
	if !ok {
		return false
	}
	if v != "host" {
		p.bad("bad_value", key, v, "%s: only \"host\" is understood, got %q", p.where(key), v)
		return false
	}
	return true
}

func (p *parser) command(n *yaml.Node) []string {
	n = p.deref(n)
	if n == nil {
		return nil
	}
	if n.Kind == yaml.SequenceNode {
		return p.list(n, "command")
	}
	s, ok := p.scalar(n, "command")
	if !ok {
		return nil
	}
	args, err := shellSplit(s)
	if err != nil {
		p.bad("bad_value", "command", err.Error(), "%s: %v", p.where("command"), err)
		return nil
	}
	return args
}

// shellSplit splits a command line like a shell would (quotes and
// backslashes only; no expansion — Docker doesn't expand either).
func shellSplit(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else {
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote, in = c, true
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			in = true
		case c == ' ' || c == '\t' || c == '\n':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteByte(c)
			in = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("the command has an unclosed quote")
	}
	if in {
		out = append(out, cur.String())
	}
	return out, nil
}

func (p *parser) environment(n *yaml.Node) []Pair {
	n = p.deref(n)
	if n == nil {
		return nil
	}
	var out []Pair
	switch n.Kind {
	case yaml.MappingNode:
		m := p.asMap(n, "environment")
		if m == nil {
			return nil
		}
		for _, k := range m.keys {
			if !envKeyRe.MatchString(k) {
				p.bad("bad_value", "environment", k, "%s: %q is not a valid variable name", p.where("environment"), k)
				continue
			}
			v, ok := p.scalar(m.vals[k], "environment")
			if ok {
				out = append(out, Pair{k, v})
			}
		}
	case yaml.SequenceNode:
		for _, e := range p.list(n, "environment") {
			k, v, found := strings.Cut(e, "=")
			if !found || !envKeyRe.MatchString(k) {
				p.bad("bad_value", "environment", e, "%s: %q must be NAME=value", p.where("environment"), e)
				continue
			}
			out = append(out, Pair{k, v})
		}
	default:
		p.bad("bad_value", "environment", "", "%s must be a list or a mapping", p.where("environment"))
	}
	return out
}

func (p *parser) ports(n *yaml.Node) []PortSpec {
	n = p.deref(n)
	if n == nil || n.Kind != yaml.SequenceNode {
		p.bad("bad_value", "ports", "", "%s must be a list", p.where("ports"))
		return nil
	}
	var out []PortSpec
	for _, e := range n.Content {
		e = p.deref(e)
		if e == nil {
			continue
		}
		switch e.Kind {
		case yaml.ScalarNode:
			s, ok := p.scalar(e, "ports")
			if !ok {
				continue
			}
			if ps, ok := p.shortPort(s); ok {
				out = append(out, ps)
			}
		case yaml.MappingNode:
			m := p.asMap(e, "ports")
			if m == nil {
				continue
			}
			var ps PortSpec
			for _, k := range m.keys {
				v, _ := p.scalar(m.vals[k], "ports")
				switch k {
				case "target":
					ps.Target = v
				case "published":
					ps.Published = v
				case "protocol":
					ps.Protocol = v
				case "host_ip":
					ps.HostIP = v
				case "mode", "name", "app_protocol":
				default:
					p.bad("unsupported_key", "ports."+k, "", "%s: %q is not supported", p.where("ports"), k)
				}
			}
			out = append(out, ps)
		default:
			p.bad("bad_value", "ports", "", "%s: an entry must be text or a mapping", p.where("ports"))
		}
	}
	return out
}

// shortPort reads "[ip:][host:]container[/proto]". The pieces stay text
// (they may hold ${…}); numbers are checked when the file is resolved. An
// IPv6 literal would need brackets and is not understood.
func (p *parser) shortPort(s string) (PortSpec, bool) {
	spec := PortSpec{}
	body, proto, hasProto := strings.Cut(s, "/")
	if hasProto {
		spec.Protocol = proto
	}
	// ${…:-…} defaults contain ':' — protect them while splitting.
	parts := splitOutsideRefs(body, ':')
	switch len(parts) {
	case 1:
		spec.Target = parts[0]
	case 2:
		spec.Published, spec.Target = parts[0], parts[1]
	case 3:
		spec.HostIP, spec.Published, spec.Target = parts[0], parts[1], parts[2]
	default:
		p.bad("port_invalid", "ports", s, "%s: %q is not a port mapping", p.where("ports"), s)
		return spec, false
	}
	return spec, true
}

// splitOutsideRefs splits s at sep, except inside ${…}.
func splitOutsideRefs(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '{':
			depth++
			i++
		case s[i] == '}' && depth > 0:
			depth--
		case s[i] == sep && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func (p *parser) volumes(n *yaml.Node) []VolumeSpec {
	n = p.deref(n)
	if n == nil || n.Kind != yaml.SequenceNode {
		p.bad("bad_value", "volumes", "", "%s must be a list", p.where("volumes"))
		return nil
	}
	var out []VolumeSpec
	for _, e := range n.Content {
		e = p.deref(e)
		if e == nil {
			continue
		}
		switch e.Kind {
		case yaml.ScalarNode:
			s, ok := p.scalar(e, "volumes")
			if !ok {
				continue
			}
			parts := splitOutsideRefs(s, ':')
			v := VolumeSpec{}
			switch len(parts) {
			case 1:
				p.bad("volume_anonymous", "volumes", s, "%s: %q has no source; give the folder a name or a path so it is kept in the app's storage", p.where("volumes"), s)
				continue
			case 2, 3:
				v.Source, v.Target = parts[0], parts[1]
				if len(parts) == 3 {
					for _, o := range strings.Split(parts[2], ",") {
						switch o {
						case "ro":
							v.ReadOnly = true
						case "rw", "z", "Z":
						default:
							p.bad("volume_invalid", "volumes", s, "%s: the option %q in %q is not supported (only ro and rw)", p.where("volumes"), o, s)
						}
					}
				}
			default:
				p.bad("volume_invalid", "volumes", s, "%s: %q is not a volume mapping", p.where("volumes"), s)
				continue
			}
			out = append(out, v)
		case yaml.MappingNode:
			m := p.asMap(e, "volumes")
			if m == nil {
				continue
			}
			v := VolumeSpec{}
			typ := "volume"
			for _, k := range m.keys {
				val, _ := p.scalar(m.vals[k], "volumes")
				switch k {
				case "type":
					typ = val
				case "source":
					v.Source = val
				case "target":
					v.Target = val
				case "read_only":
					v.ReadOnly = val == "true"
				default:
					p.bad("unsupported_key", "volumes."+k, "", "%s: %q is not supported", p.where("volumes"), k)
				}
			}
			if typ != "bind" && typ != "volume" {
				p.bad("volume_invalid", "volumes", typ, "%s: the volume type %q is not supported (bind or named volume)", p.where("volumes"), typ)
				continue
			}
			if v.Source == "" {
				p.bad("volume_anonymous", "volumes", v.Target, "%s: the volume at %q has no source", p.where("volumes"), v.Target)
				continue
			}
			out = append(out, v)
		default:
			p.bad("bad_value", "volumes", "", "%s: an entry must be text or a mapping", p.where("volumes"))
		}
	}
	return out
}

// deploy reads deploy.resources.limits.memory; anything else under deploy
// is refused.
func (p *parser) deploy(n *yaml.Node, cur string) string {
	m := p.asMap(n, "deploy")
	if m == nil {
		return cur
	}
	for _, k := range m.keys {
		if k != "resources" {
			p.bad("unsupported_key", "deploy."+k, "", "service %s: deploy.%s is not supported (only deploy.resources.limits.memory)", p.svc, k)
			continue
		}
		res := p.asMap(m.vals[k], "deploy.resources")
		if res == nil {
			continue
		}
		for _, rk := range res.keys {
			if rk != "limits" {
				p.bad("unsupported_key", "deploy.resources."+rk, "", "service %s: deploy.resources.%s is not supported (only limits.memory)", p.svc, rk)
				continue
			}
			lim := p.asMap(res.vals[rk], "deploy.resources.limits")
			if lim == nil {
				continue
			}
			for _, lk := range lim.keys {
				if lk != "memory" {
					p.bad("unsupported_key", "deploy.resources.limits."+lk, "", "service %s: limits.%s is not supported (only memory)", p.svc, lk)
					continue
				}
				cur, _ = p.scalar(lim.vals[lk], "deploy.resources.limits.memory")
			}
		}
	}
	return cur
}

func (p *parser) dependsOn(n *yaml.Node) []string {
	n = p.deref(n)
	if n == nil {
		return nil
	}
	if n.Kind == yaml.MappingNode {
		m := p.asMap(n, "depends_on")
		if m == nil {
			return nil
		}
		return m.keys
	}
	return p.list(n, "depends_on")
}

// order puts the services in start order and checks depends_on.
func (p *parser) order() {
	by := map[string]*Service{}
	for _, s := range p.f.Services {
		by[s.Name] = s
	}
	for _, s := range p.f.Services {
		for _, d := range s.DependsOn {
			if by[d] == nil {
				p.svc = s.Name
				p.bad("depends_unknown", "depends_on", d, "service %s depends on %q, which isn't in the file", s.Name, d)
			}
		}
	}
	p.svc = ""
	if len(p.issue) > 0 {
		return
	}
	var out []*Service
	state := map[string]int{} // 1 visiting, 2 done
	var visit func(s *Service) bool
	visit = func(s *Service) bool {
		switch state[s.Name] {
		case 1:
			return false
		case 2:
			return true
		}
		state[s.Name] = 1
		for _, d := range s.DependsOn {
			if !visit(by[d]) {
				return false
			}
		}
		state[s.Name] = 2
		out = append(out, s)
		return true
	}
	for _, s := range p.f.Services {
		if !visit(s) {
			p.bad("depends_cycle", "depends_on", s.Name, "the services depend on each other in a circle")
			return
		}
	}
	p.f.Services = out
}

// hints adds a note where a service name is used as a host name: on the
// box's own network the containers reach each other on 127.0.0.1.
func (p *parser) hints() {
	if len(p.f.Services) < 2 {
		return
	}
	for _, s := range p.f.Services {
		for _, e := range s.Env {
			for _, o := range p.f.Services {
				if o.Name != s.Name && strings.EqualFold(strings.TrimSpace(e.Value), o.Name) {
					p.svc = s.Name
					p.note("service_hostname", e.Key, o.Name, "service %s: %s refers to the service %q by name; on this box all containers share one network, so use 127.0.0.1", s.Name, e.Key, o.Name)
				}
			}
		}
	}
	p.svc = ""
}

// MainService is the service that carries the web UI and the app's name:
// x-dreamhub.main, else the first (in start order) that no other service
// depends on, else the last one.
func (f *File) MainService() *Service {
	if f.Ext.Main != "" {
		for _, s := range f.Services {
			if s.Name == f.Ext.Main {
				return s
			}
		}
	}
	needed := map[string]bool{}
	for _, s := range f.Services {
		for _, d := range s.DependsOn {
			needed[d] = true
		}
	}
	for _, s := range f.Services {
		if !needed[s.Name] {
			return s
		}
	}
	return f.Services[len(f.Services)-1]
}
