package compose

import (
	"fmt"
	"strings"
)

// Variable references use the compose spec's syntax: $$ is a literal $,
// $NAME and ${NAME} read a variable, ${NAME:-default} / ${NAME-default}
// give a default (":" also covers an empty value), ${NAME:?message} /
// ${NAME?message} make it required. ${NAME:+x}, nested references and
// anything else are refused — a form field has one name, one default.

// Var is one variable a compose file refers to: an input field of the form.
type Var struct {
	Name       string `json:"name"`
	Default    string `json:"default,omitempty"`
	HasDefault bool   `json:"hasDefault,omitempty"`
	// Required: no default, or the file says ${NAME:?…}.
	Required bool `json:"required,omitempty"`
}

// ref is one parsed reference in a string.
type ref struct {
	name       string
	def        string
	hasDefault bool
	colon      bool // ":-" / ":?" (empty counts as unset)
	required   bool
	message    string
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isNameChar(c byte) bool { return isNameStart(c) || (c >= '0' && c <= '9') }

// segment is a literal run or a reference.
type segment struct {
	lit string
	ref *ref
}

// splitRefs parses s into literals and references.
func splitRefs(s string) ([]segment, error) {
	var out []segment
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			out = append(out, segment{lit: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '$' {
			lit.WriteByte(c)
			continue
		}
		if i+1 >= len(s) {
			return nil, fmt.Errorf("a lone $ at the end of %q (write $$ for a literal $)", s)
		}
		n := s[i+1]
		switch {
		case n == '$':
			lit.WriteByte('$')
			i++
		case isNameStart(n):
			j := i + 1
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			flush()
			out = append(out, segment{ref: &ref{name: s[i+1 : j], required: true}})
			i = j - 1
		case n == '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return nil, fmt.Errorf("unclosed ${ in %q", s)
			}
			body := s[i+2 : i+end]
			// $$ inside a default is a literal $; any other $ is a nested
			// reference.
			if strings.Contains(strings.ReplaceAll(body, "$$", ""), "$") {
				return nil, fmt.Errorf("nested variables are not supported in %q", s)
			}
			j := 0
			for j < len(body) && isNameChar(body[j]) {
				j++
			}
			if j == 0 || !isNameStart(body[0]) {
				return nil, fmt.Errorf("invalid variable name in ${%s}", body)
			}
			r := &ref{name: body[:j]}
			rest := body[j:]
			switch {
			case rest == "":
				r.required = true
			case strings.HasPrefix(rest, ":-"):
				r.colon, r.hasDefault, r.def = true, true, strings.ReplaceAll(rest[2:], "$$", "$")
			case strings.HasPrefix(rest, "-"):
				r.hasDefault, r.def = true, strings.ReplaceAll(rest[1:], "$$", "$")
			case strings.HasPrefix(rest, ":?"):
				r.colon, r.required, r.message = true, true, rest[2:]
			case strings.HasPrefix(rest, "?"):
				r.required, r.message = true, rest[1:]
			default:
				return nil, fmt.Errorf("unsupported variable form ${%s}", body)
			}
			flush()
			out = append(out, segment{ref: r})
			i += end
		default:
			return nil, fmt.Errorf("a $ that is not a variable in %q (write $$ for a literal $)", s)
		}
	}
	flush()
	return out, nil
}

// vars collects the variables of a file in the order of first use.
type vars struct {
	order []string
	by    map[string]*Var
}

func newVars() *vars { return &vars{by: map[string]*Var{}} }

func (v *vars) add(r *ref) {
	cur, ok := v.by[r.name]
	if !ok {
		nv := &Var{Name: r.name, Default: r.def, HasDefault: r.hasDefault, Required: r.required || !r.hasDefault}
		v.by[r.name] = nv
		v.order = append(v.order, r.name)
		return
	}
	// The first default wins; a later one only fills a gap.
	if !cur.HasDefault && r.hasDefault {
		cur.Default, cur.HasDefault = r.def, true
	}
	if cur.HasDefault {
		cur.Required = false
	}
}

func (v *vars) list() []Var {
	out := make([]Var, 0, len(v.order))
	for _, n := range v.order {
		out = append(out, *v.by[n])
	}
	return out
}

// substitute replaces every reference in s from values (a name missing
// from values falls back to the file's default, then fails).
func substitute(s string, values map[string]string, defs map[string]*Var) (string, error) {
	segs, err := splitRefs(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, sg := range segs {
		if sg.ref == nil {
			b.WriteString(sg.lit)
			continue
		}
		val, ok := values[sg.ref.name]
		if !ok || val == "" {
			if d := defs[sg.ref.name]; d != nil && d.HasDefault {
				val, ok = d.Default, true
			}
		}
		if !ok || val == "" {
			if d := defs[sg.ref.name]; d != nil && d.Required {
				return "", fmt.Errorf("a value for %s is required", sg.ref.name)
			}
		}
		b.WriteString(val)
	}
	return b.String(), nil
}
