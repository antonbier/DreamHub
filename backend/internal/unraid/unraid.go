// Package unraid turns the XML template of an Unraid Docker container
// (<Container version="2">) into a DreamHub compose file (docs/APPSHUB_PLAN.md,
// App12). It is pure parsing and rewriting: no network, no files, no Docker.
//
// DreamHub bundles no Unraid templates. The format is publicly documented,
// but the templates' texts and icons belong to their authors and "Unraid" is
// a trademark; so an admin pastes a template they chose and this package
// builds the same install form from it as from a compose file. (Not legal
// advice; the design avoids the question.)
//
// What is taken over: Repository (image and tag), Name, Overview, WebUI
// (port and path), Privileged, and the Config entries of type Port, Path and
// Variable. What is left out, with a note each: Device and Label entries,
// ExtraParams and PostArgs (never interpreted — they are free-form docker
// arguments, which DreamHub's helper never takes), the icon (the UI makes no
// external requests), the network mode (apps use the box's network), and host
// port mappings that differ from the container port (the app listens on its
// container port itself).
package unraid

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/antonbier/DreamHub/backend/internal/compose"
)

// MaxSize bounds the template Convert reads.
const MaxSize = 64 << 10

type container struct {
	XMLName    xml.Name `xml:"Container"`
	Version    string   `xml:"version,attr"`
	Name       string   `xml:"Name"`
	Repository string   `xml:"Repository"`
	Network    string   `xml:"Network"`
	Privileged string   `xml:"Privileged"`
	Overview   string   `xml:"Overview"`
	WebUI      string   `xml:"WebUI"`
	Icon       string   `xml:"Icon"`
	ExtraParam string   `xml:"ExtraParams"`
	PostArgs   string   `xml:"PostArgs"`
	Configs    []config `xml:"Config"`
}

type config struct {
	Name        string `xml:"Name,attr"`
	Target      string `xml:"Target,attr"`
	Default     string `xml:"Default,attr"`
	Mode        string `xml:"Mode,attr"`
	Description string `xml:"Description,attr"`
	Type        string `xml:"Type,attr"`
	Required    string `xml:"Required,attr"`
	Mask        string `xml:"Mask,attr"`
	Value       string `xml:",chardata"`
}

func note(code, detail, format string, args ...any) compose.Issue {
	return compose.Issue{Code: code, Detail: detail, Message: fmt.Sprintf(format, args...)}
}

func refuse(code, detail, format string, args ...any) compose.Issues {
	return compose.Issues{note(code, detail, format, args...)}
}

var (
	webUIRe    = regexp.MustCompile(`\[PORT:(\d{1,5})\](.*)$`)
	bbcodeRe   = regexp.MustCompile(`\[/?[a-zA-Z]+(=[^\]]*)?\]`)
	identRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	nonAlnumRe = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// Convert reads an Unraid template and returns the compose file text and
// the notes about what was left out.
func Convert(data []byte) (string, []compose.Issue, error) {
	if len(data) > MaxSize {
		return "", nil, refuse("too_large", "", "the template is larger than %d KB", MaxSize>>10)
	}
	var c container
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = true
	if err := dec.Decode(&c); err != nil {
		return "", nil, refuse("unraid_xml", err.Error(), "this is not an Unraid template: %v", err)
	}
	if c.XMLName.Local != "Container" {
		return "", nil, refuse("unraid_xml", "", "an Unraid template starts with <Container>")
	}
	repo := strings.TrimSpace(c.Repository)
	image, tag, err := compose.SplitImage(repo)
	if err != nil {
		return "", nil, refuse("image_invalid", repo, "the repository %q is not usable: %v", repo, err)
	}
	if tag != "" {
		image += ":" + tag
	}

	var notes []compose.Issue
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = image
	}

	ext := mapping()
	add(ext, "name", scalar(name))
	if ov := cleanOverview(c.Overview); ov != "" {
		add(ext, "description", mapping("en", scalar(ov)))
	}
	if m := webUIRe.FindStringSubmatch(c.WebUI); m != nil {
		web := mapping("port", scalar(m[1]))
		if p := strings.TrimSpace(m[2]); p != "" && strings.HasPrefix(p, "/") {
			add(web, "path", scalar(p))
		}
		add(ext, "webui", web)
	}
	if strings.TrimSpace(c.Icon) != "" {
		notes = append(notes, note("unraid_icon", "", "the template's icon is not loaded (DreamHub makes no external requests from its pages)"))
	}

	svc := mapping("image", scalar(image))
	if strings.EqualFold(strings.TrimSpace(c.Privileged), "true") {
		add(svc, "privileged", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		notes = append(notes, note("unraid_privileged", "", "the template runs the container privileged; installing it will ask for an extra confirmation"))
	}
	if n := strings.ToLower(strings.TrimSpace(c.Network)); n != "" && n != "host" && n != "bridge" {
		notes = append(notes, note("unraid_network", c.Network, "the network %q is ignored; apps run on the box's own network", c.Network))
	}
	if strings.TrimSpace(c.ExtraParam) != "" || strings.TrimSpace(c.PostArgs) != "" {
		notes = append(notes, note("unraid_extraparams", strings.TrimSpace(c.ExtraParam+" "+c.PostArgs), "the extra docker parameters (%s) are not used; DreamHub never passes free-form docker arguments", strings.TrimSpace(c.ExtraParam+" "+c.PostArgs)))
	}

	fields := mapping()
	env := mapping()
	var ports, volumes []*yaml.Node
	usedVars := map[string]bool{}
	usedFolders := map[string]bool{}
	var ignored []string
	remapped := false
	hostPaths := false

	for i, cfg := range c.Configs {
		value := strings.TrimSpace(cfg.Value)
		def := cfg.Default
		if value == "" {
			value = def
		}
		label := strings.TrimSpace(cfg.Name)
		switch strings.ToLower(cfg.Type) {
		case "port":
			target, err := strconv.Atoi(strings.TrimSpace(cfg.Target))
			if err != nil || target < 1 || target > 65535 {
				notes = append(notes, note("unraid_skipped", label, "the port entry %q has no usable container port and was left out", label))
				continue
			}
			spec := strconv.Itoa(target)
			if strings.EqualFold(strings.TrimSpace(cfg.Mode), "udp") {
				spec += "/udp"
			}
			if host, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && host != target {
				remapped = true
			}
			ports = append(ports, scalar(spec))
		case "path":
			target := strings.TrimSpace(cfg.Target)
			if !strings.HasPrefix(target, "/") {
				notes = append(notes, note("unraid_skipped", label, "the path entry %q has no container path and was left out", label))
				continue
			}
			folder := strings.ToLower(nonAlnumRe.ReplaceAllString(label, ""))
			if folder == "" || len(folder) > 20 || folder == "secrets" {
				folder = "folder" + strconv.Itoa(i+1)
			}
			for usedFolders[folder] {
				folder = "folder" + strconv.Itoa(i+1)
				if usedFolders[folder] {
					folder += "x"
				}
			}
			usedFolders[folder] = true
			v := "PATH_" + strings.ToUpper(folder)
			usedVars[v] = true
			src := "${" + v + ":-./" + folder + "}"
			if strings.EqualFold(strings.TrimSpace(cfg.Mode), "ro") {
				volumes = append(volumes, scalar(src+":"+target+":ro"))
			} else {
				volumes = append(volumes, scalar(src+":"+target))
			}
			hostPaths = true
			meta := mapping("label", scalar(orDefault(label, target)), "type", scalar("path"))
			if d := strings.TrimSpace(cfg.Description); d != "" {
				add(meta, "description", scalar(d))
			}
			add(fields, v, meta)
		case "variable":
			key := strings.TrimSpace(cfg.Target)
			if !identRe.MatchString(key) {
				notes = append(notes, note("unraid_skipped", label, "the variable %q has a name that can't be used and was left out", key))
				continue
			}
			if usedVars[key] {
				continue
			}
			usedVars[key] = true
			required := strings.EqualFold(cfg.Required, "true") && value == ""
			ref := "${" + key + ":-" + escape(value) + "}"
			if required {
				ref = "${" + key + "}"
			}
			add(env, key, scalar(ref))
			meta := mapping("label", scalar(orDefault(label, key)))
			if d := strings.TrimSpace(cfg.Description); d != "" {
				add(meta, "description", scalar(d))
			}
			if strings.EqualFold(cfg.Mask, "true") {
				add(meta, "secret", boolNode(true))
			}
			add(fields, key, meta)
		case "device":
			ignored = append(ignored, "device "+orDefault(cfg.Target, label))
		case "label":
			ignored = append(ignored, "label "+orDefault(cfg.Target, label))
		default:
			ignored = append(ignored, strings.TrimSpace(cfg.Type+" "+label))
		}
	}
	if len(ignored) > 0 {
		notes = append(notes, note("unraid_ignored", strings.Join(ignored, ", "), "left out: %s (devices and labels are not imported)", strings.Join(ignored, ", ")))
	}
	if remapped {
		notes = append(notes, note("unraid_port_remap", "", "Unraid maps host ports to container ports; apps here use the box's own network, so each app listens on its container port itself"))
	}
	if hostPaths {
		notes = append(notes, note("unraid_paths", "", "the template's folders are replaced by folders in the app's storage; pick real folders below /media in the form if the app needs your files"))
	}

	if len(env.Content) > 0 {
		add(svc, "environment", env)
	}
	if len(ports) > 0 {
		add(svc, "ports", &yaml.Node{Kind: yaml.SequenceNode, Content: ports})
	}
	if len(volumes) > 0 {
		add(svc, "volumes", &yaml.Node{Kind: yaml.SequenceNode, Content: volumes})
	}
	if len(fields.Content) > 0 {
		add(ext, "fields", fields)
	}
	doc := mapping("x-dreamhub", ext, "services", mapping("app", svc))
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", nil, refuse("unraid_xml", err.Error(), "the template could not be converted: %v", err)
	}
	// What we produce must be something compose.Parse accepts; a template
	// that makes it refuse is refused here with that reason.
	if _, err := compose.Parse(out); err != nil {
		return "", nil, err
	}
	return string(out), notes, nil
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return strings.TrimSpace(s)
}

// escape protects a literal inside ${NAME:-default}: $ becomes $$ and a
// closing brace can't be written, so it is dropped.
func escape(s string) string {
	s = strings.ReplaceAll(s, "$", "$$")
	return strings.ReplaceAll(s, "}", "")
}

func cleanOverview(s string) string {
	s = bbcodeRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 400 {
		s = strings.TrimSpace(s[:400]) + "…"
	}
	return s
}

// --- small helpers to build an ordered YAML tree ---

func scalar(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

func boolNode(b bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(b)}
}

// mapping builds an ordered mapping from key, node pairs.
func mapping(kv ...any) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for i := 0; i+1 < len(kv); i += 2 {
		add(n, kv[i].(string), kv[i+1].(*yaml.Node))
	}
	return n
}

func add(m *yaml.Node, key string, v *yaml.Node) {
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
}
