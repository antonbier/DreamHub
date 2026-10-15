package compose

import (
	"regexp"

	"go.yaml.in/yaml/v3"
)

var (
	iconRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	langRe = regexp.MustCompile(`^[a-z]{2}$`)
	arches = regexp.MustCompile(`^[a-z0-9]+(/[a-z0-9]+)?$`)
)

// text reads a string or a mapping of language → string.
func (p *parser) text(n *yaml.Node, key string) Text {
	n = p.deref(n)
	if n == nil {
		return nil
	}
	if n.Kind == yaml.ScalarNode {
		return Text{"en": n.Value}
	}
	m := p.asMap(n, key)
	if m == nil {
		return nil
	}
	t := Text{}
	for _, k := range m.keys {
		if !langRe.MatchString(k) {
			p.bad("ext_invalid", key, k, "x-dreamhub: %s has %q, which is not a language code", key, k)
			continue
		}
		v := p.deref(m.vals[k])
		if v == nil || v.Kind != yaml.ScalarNode {
			p.bad("ext_invalid", key, k, "x-dreamhub: %s.%s must be text", key, k)
			continue
		}
		t[k] = v.Value
	}
	return t
}

// plain reads a scalar of x-dreamhub without interpolation.
func (p *parser) plain(n *yaml.Node, key string) string {
	n = p.deref(n)
	if n == nil || n.Kind != yaml.ScalarNode {
		p.bad("ext_invalid", key, "", "x-dreamhub: %s must be a single value", key)
		return ""
	}
	return n.Value
}

func (p *parser) flag(n *yaml.Node, key string) bool {
	n = p.deref(n)
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag != "!!bool" {
		p.bad("ext_invalid", key, "", "x-dreamhub: %s must be true or false", key)
		return false
	}
	return n.Value == "true"
}

// ext reads the top-level x-dreamhub block.
func (p *parser) ext(n *yaml.Node) {
	saved := p.svc
	p.svc = ""
	defer func() { p.svc = saved }()
	m := p.asMap(n, "x-dreamhub")
	if m == nil {
		return
	}
	e := &p.f.Ext
	e.Fields = map[string]FieldMeta{}
	for _, k := range m.keys {
		v := m.vals[k]
		switch k {
		case "name":
			e.Name = p.plain(v, k)
		case "icon":
			e.Icon = p.plain(v, k)
		case "category":
			e.Category = p.plain(v, k)
		case "description":
			e.Description = p.text(v, k)
		case "note":
			e.Note = p.text(v, k)
		case "main":
			e.Main = p.plain(v, k)
		case "largeConfig":
			e.LargeConfig = p.flag(v, k)
		case "webui":
			w := p.asMap(v, "webui")
			if w == nil {
				continue
			}
			for _, wk := range w.keys {
				switch wk {
				case "port":
					if s, ok := p.scalar(w.vals[wk], "webui.port"); ok {
						e.WebPort = s
					}
				case "path":
					e.WebPath = p.plain(w.vals[wk], "webui.path")
				default:
					p.bad("ext_invalid", "webui."+wk, "", "x-dreamhub: webui.%s is not known (port, path)", wk)
				}
			}
		case "fields":
			fm := p.asMap(v, "fields")
			if fm == nil {
				continue
			}
			for _, name := range fm.keys {
				e.Fields[name] = p.fieldMeta(name, fm.vals[name])
				e.FieldOrder = append(e.FieldOrder, name)
			}
		default:
			p.bad("ext_invalid", k, "", "x-dreamhub: %q is not known", k)
		}
	}
}

func (p *parser) fieldMeta(name string, n *yaml.Node) FieldMeta {
	var fm FieldMeta
	m := p.asMap(n, "fields."+name)
	if m == nil {
		return fm
	}
	for _, k := range m.keys {
		v := m.vals[k]
		switch k {
		case "label":
			fm.Label = p.text(v, "fields."+name+".label")
		case "description":
			fm.Description = p.text(v, "fields."+name+".description")
		case "required":
			fm.Required = p.flag(v, "fields."+name+".required")
		case "secret":
			fm.Secret = p.flag(v, "fields."+name+".secret")
		case "advanced":
			adv := p.flag(v, "fields."+name+".advanced")
			fm.Advanced = &adv
		case "type":
			fm.Type = p.plain(v, "fields."+name+".type")
			switch fm.Type {
			case "port", "path", "number", "text":
			default:
				p.bad("ext_invalid", "fields."+name+".type", fm.Type, "x-dreamhub: the type of field %s must be port, path, number or text", name)
			}
		default:
			p.bad("ext_invalid", "fields."+name+"."+k, "", "x-dreamhub: field %s has %q, which is not known", name, k)
		}
	}
	return fm
}

// serviceExt reads a service's own x-dreamhub block (the verified
// architectures of its image).
func (p *parser) serviceExt(s *Service, n *yaml.Node) {
	m := p.asMap(n, "x-dreamhub")
	if m == nil {
		return
	}
	for _, k := range m.keys {
		if k != "arches" {
			p.bad("ext_invalid", "x-dreamhub."+k, "", "service %s: x-dreamhub.%s is not known (arches)", s.Name, k)
			continue
		}
		for _, a := range p.list(m.vals[k], "x-dreamhub.arches") {
			if !arches.MatchString(a) {
				p.bad("ext_invalid", "x-dreamhub.arches", a, "service %s: %q is not an architecture like amd64 or arm/v7", s.Name, a)
				continue
			}
			s.Arches = append(s.Arches, a)
		}
	}
}

// checkExt checks the x-dreamhub block against the rest of the file.
func (p *parser) checkExt() {
	e := &p.f.Ext
	if e.Icon != "" && !iconRe.MatchString(e.Icon) {
		p.bad("ext_invalid", "icon", e.Icon, "x-dreamhub: the icon %q must be a short lowercase name", e.Icon)
	}
	if e.Main != "" {
		found := false
		for _, s := range p.f.Services {
			found = found || s.Name == e.Main
		}
		if !found {
			p.bad("ext_invalid", "main", e.Main, "x-dreamhub: main names the service %q, which isn't in the file", e.Main)
		}
	}
	if e.WebPath != "" && (e.WebPath[0] != '/' || len(e.WebPath) > 200) {
		p.bad("ext_invalid", "webui.path", e.WebPath, "x-dreamhub: webui.path must start with /")
	}
	for _, name := range e.FieldOrder {
		if _, used := p.vars.by[name]; !used {
			p.bad("ext_invalid", "fields."+name, name, "x-dreamhub: the field %q is not used by any ${%s} in the file", name, name)
		}
	}
}
