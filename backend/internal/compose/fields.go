package compose

import "strings"

// Field is one input of the install form: a variable of the compose file
// plus what x-dreamhub says about it.
type Field struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	// Type is "port", "path", "number" or "text".
	Type     string `json:"type"`
	Default  string `json:"default,omitempty"`
	Required bool   `json:"required,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
	// Advanced fields are only shown in the form's advanced view. The web
	// port and required fields are always shown.
	Advanced bool `json:"advanced,omitempty"`
	// WebPort marks the field holding the web UI port.
	WebPort bool `json:"webPort,omitempty"`
}

// secretish names get a masked input unless x-dreamhub says otherwise.
var secretWords = []string{"PASSWORD", "PASSWD", "SECRET", "TOKEN", "APIKEY", "API_KEY"}

// Fields lists the form fields in display order: the web port first, then
// the fields x-dreamhub lists (in its order), then the rest in the order
// the file uses them. lang picks labels and descriptions.
func (f *File) Fields(lang string) []Field {
	roles := f.varRoles()
	webVar := ""
	if refs, err := splitRefs(f.Ext.WebPort); err == nil {
		for _, sg := range refs {
			if sg.ref != nil {
				webVar = sg.ref.name
				break
			}
		}
	}
	byName := map[string]Var{}
	for _, v := range f.Vars {
		byName[v.Name] = v
	}
	var order []string
	seen := map[string]bool{}
	add := func(n string) {
		if _, ok := byName[n]; ok && !seen[n] {
			seen[n] = true
			order = append(order, n)
		}
	}
	add(webVar)
	for _, n := range f.Ext.FieldOrder {
		add(n)
	}
	for _, v := range f.Vars {
		add(v.Name)
	}
	out := make([]Field, 0, len(order))
	for _, n := range order {
		v := byName[n]
		meta := f.Ext.Fields[n]
		fl := Field{
			Name: n, Label: meta.Label.Get(lang), Description: meta.Description.Get(lang),
			Type: meta.Type, Default: v.Default, Required: v.Required || meta.Required,
			Secret: meta.Secret, Advanced: meta.Advanced == nil || *meta.Advanced, WebPort: n == webVar,
		}
		if fl.Label == "" {
			fl.Label = n
		}
		if fl.Type == "" {
			fl.Type = roles[n]
			if fl.Type == "" {
				fl.Type = "text"
			}
		}
		if !meta.Secret {
			up := strings.ToUpper(n)
			for _, w := range secretWords {
				if strings.Contains(up, w) {
					fl.Secret = true
				}
			}
		}
		// The simple view shows the web port and the required fields (and
		// any field the file pins there with advanced: false).
		if fl.WebPort || fl.Required {
			fl.Advanced = false
		}
		out = append(out, fl)
	}
	return out
}

// varRoles infers a field type from where each variable is used: in a port
// or the web UI port → "port", as a volume source → "path".
func (f *File) varRoles() map[string]string {
	roles := map[string]string{}
	mark := func(s, role string) {
		segs, err := splitRefs(s)
		if err != nil {
			return
		}
		for _, sg := range segs {
			if sg.ref != nil && roles[sg.ref.name] == "" {
				roles[sg.ref.name] = role
			}
		}
	}
	mark(f.Ext.WebPort, "port")
	for _, s := range f.Services {
		for _, p := range s.Ports {
			mark(p.Published, "port")
			mark(p.Target, "port")
		}
		for _, v := range s.Volumes {
			mark(v.Source, "path")
		}
	}
	return roles
}
