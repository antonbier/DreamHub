package hubs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/compose"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// This file is AppsHub's App13 "edit" (docs/APPSHUB_PLAN.md): the install
// form reopened for an installed app, filled with what was chosen, applied
// as a plan that makes the app's containers again. Its folders are never
// touched, so its data stays.
//
// What was chosen is kept next to the app's other files, in the storage root
// (<root>/<id>.app.json, readable by DreamHub only, outside every container's
// mount — like the generated passwords): the compose file of a custom app,
// the form values, the extra rows and the memory limit. Secret values are
// never sent back to the browser; an empty secret field on edit means "keep".

// appDefinition is what is stored per app.
type appDefinition struct {
	Version       int               `json:"version"`
	AppID         string            `json:"appId,omitempty"`
	Custom        *customBody       `json:"custom,omitempty"`
	Values        map[string]string `json:"values,omitempty"`
	Extra         *extraBody        `json:"extra,omitempty"`
	MemoryLimitMB int               `json:"memoryLimitMB,omitempty"`
}

func definitionFile(root, appID string) string { return path.Join(root, appID+".app.json") }

func definitionOf(b installBody) appDefinition {
	d := appDefinition{Version: 1, AppID: b.AppID, Custom: b.Custom, Values: b.Values, Extra: b.Extra, MemoryLimitMB: b.MemoryLimitMB}
	if b.Extra != nil && len(b.Extra.Env)+len(b.Extra.Ports)+len(b.Extra.Paths) == 0 {
		d.Extra = nil
	}
	return d
}

// saveDefinition is best effort: an app whose choices could not be stored is
// installed all the same, only its edit form then starts from the defaults.
func (h *Apps) saveDefinition(root, appID string, d appDefinition) {
	data, err := json.Marshal(d)
	if err != nil {
		return
	}
	_ = h.configFS().WriteFile(definitionFile(root, appID), data)
}

func (h *Apps) loadDefinition(root, appID string) (appDefinition, bool) {
	data, err := h.configFS().ReadFile(definitionFile(root, appID))
	if err != nil {
		return appDefinition{}, false
	}
	var d appDefinition
	if json.Unmarshal(data, &d) != nil || d.Version != 1 {
		return appDefinition{}, false
	}
	return d, true
}

// body is the install request that produces the stored definition again.
func (d appDefinition) body(appID string) installBody {
	return installBody{AppID: d.AppID, Custom: d.Custom, Values: d.Values, Extra: d.Extra, MemoryLimitMB: d.MemoryLimitMB, ConfirmElevated: true, editID: appID}
}

// fieldsOf are the form fields of what the request installs.
func fieldsOf(b installBody) []compose.Field {
	switch {
	case b.AppID != "":
		if a, ok := appcatalog.ByID(b.AppID); ok {
			return a.Fields("en")
		}
	case b.Custom != nil:
		if f, err := compose.Parse([]byte(b.Custom.Compose)); err == nil {
			return f.Fields("en")
		}
	}
	return nil
}

// editable reports whether the person can open the edit form: an app of
// their own needs its stored compose file.
func (h *Apps) editable(g appGroup) bool {
	if _, curated := appcatalog.ByID(g.appID); curated {
		return true
	}
	d, ok := h.loadDefinition(g.root(h.StorageRoot), g.appID)
	return ok && d.Custom != nil
}

// --- GET /api/v1/apps/installed/{id}/edit --------------------------------------

type editExtraEnv struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Keep: a secret that is set; its value stays on the box.
	Keep bool `json:"keep,omitempty"`
}

type editExtra struct {
	Env   []editExtraEnv `json:"env"`
	Ports []extraPort    `json:"ports"`
	Paths []extraPath    `json:"paths"`
}

type editStorage struct {
	Path      string `json:"path"`
	Label     string `json:"label"`
	FreeBytes int64  `json:"freeBytes"`
}

type editView struct {
	// Kind is catalog or custom; App is what the form is built from (the
	// same shapes the install form takes).
	Kind string `json:"kind"`
	App  any    `json:"app"`
	// Known: the app's choices were stored (it was installed with this
	// version or later); otherwise the form starts from the defaults.
	Known         bool              `json:"known"`
	Values        map[string]string `json:"values"`
	SecretSet     []string          `json:"secretSet"`
	Extra         editExtra         `json:"extra"`
	MemoryLimitMB int               `json:"memoryLimitMB"`
	Storage       editStorage       `json:"storage"`
}

// editForm answers what the install form needs to be pre-filled.
func (h *Apps) editForm(w http.ResponseWriter, r *http.Request) {
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	ctx := r.Context()
	containers, err := h.Docker.List(ctx)
	if err != nil {
		writeDockerError(w, r, err, "could not look up the app")
		return
	}
	g, found := findApp(containers, r.PathValue("id"))
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	root := g.root(h.StorageRoot)
	def, known := h.loadDefinition(root, g.appID)
	lang := requestLang(r)
	view := editView{Known: known, Values: map[string]string{}, SecretSet: []string{}, Extra: editExtra{Env: []editExtraEnv{}, Ports: []extraPort{}, Paths: []extraPath{}}}

	var fields []compose.Field
	if cat, curated := appcatalog.ByID(g.appID); curated {
		entry := catalogEntryOf(cat, lang, dockerhub.BoxArch())
		entry.Installed = true
		view.Kind, view.App, fields = "catalog", entry, entry.Fields
		if !known && cat.PortEditable() {
			// Installed before choices were stored: the port it runs on now.
			if hp := g.all()[0].HostPort; hp != 0 && hp != cat.ContainerPort {
				view.Values[cat.PortVar] = fmt.Sprint(hp)
			}
		}
	} else {
		if !known || def.Custom == nil {
			httpx.WriteError(w, r, http.StatusConflict, "not_editable", "this container was added before editing existed; remove it and add it again to change it")
			return
		}
		name := def.Custom.Name
		if name == "" {
			name = groupName(g, g.all()[0].Image)
		}
		parsed, ferr := h.parsedFrom(def.Custom.Compose, name, lang, nil, h.config(ctx).defaultMemoryMB())
		if ferr != nil {
			ferr.write(w, r)
			return
		}
		parsed.Name = name
		view.Kind, view.App, fields = "custom", parsed, parsed.Fields
	}

	secret := map[string]bool{}
	for _, f := range fields {
		secret[f.Name] = f.Secret
	}
	for k, v := range def.Values {
		if secret[k] {
			if v != "" {
				view.SecretSet = append(view.SecretSet, k)
			}
			continue
		}
		view.Values[k] = v
	}
	if x := def.Extra; x != nil {
		for _, e := range x.Env {
			row := editExtraEnv{Key: e.Key, Value: e.Value}
			if looksSecret(e.Key) {
				row.Value, row.Keep = "", true
			}
			view.Extra.Env = append(view.Extra.Env, row)
		}
		view.Extra.Ports = append(view.Extra.Ports, x.Ports...)
		view.Extra.Paths = append(view.Extra.Paths, x.Paths...)
	}
	view.MemoryLimitMB = def.MemoryLimitMB
	view.Storage = editStorage{Path: root, Label: path.Dir(root)}
	for _, c := range h.rawStorageChoices(ctx) {
		if c.Path == root {
			view.Storage.Label, view.Storage.FreeBytes = c.Label, c.FreeBytes
		}
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

// --- POST /api/v1/apps/installed/{id}/edit/plan -----------------------------------

// keepSecrets fills in the secrets the form left empty from what is stored:
// an empty secret field, or an extra variable row marked keep, means "as it
// is".
func keepSecrets(b *installBody, old appDefinition) {
	vals := map[string]string{}
	for k, v := range b.Values {
		vals[k] = v
	}
	for _, f := range fieldsOf(*b) {
		if f.Secret && vals[f.Name] == "" && old.Values[f.Name] != "" {
			vals[f.Name] = old.Values[f.Name]
		}
	}
	b.Values = vals
	if b.Extra == nil || old.Extra == nil {
		return
	}
	oldEnv := map[string]string{}
	for _, e := range old.Extra.Env {
		oldEnv[e.Key] = e.Value
	}
	x := *b.Extra
	x.Env = append([]extraEnv(nil), x.Env...)
	for i, e := range x.Env {
		if e.Value == "" && looksSecret(e.Key) {
			if v, ok := oldEnv[e.Key]; ok {
				x.Env[i].Value = v
			}
		}
	}
	b.Extra = &x
}

func portSetOf(app appcatalog.App) map[[2]int]bool {
	set := map[[2]int]bool{}
	add := func(p appcatalog.Port) {
		k := [2]int{p.Port, 0}
		if p.UDP {
			k[1] = 1
		}
		set[k] = true
	}
	if app.ContainerPort != 0 {
		add(appcatalog.Port{Port: app.ContainerPort})
	}
	for _, p := range app.Ports {
		add(p)
	}
	for _, s := range app.Services {
		for _, p := range s.Ports {
			add(p)
		}
	}
	return set
}

func (h *Apps) planEdit(w http.ResponseWriter, r *http.Request) {
	var body installBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"appId": "..."} or {"custom": {...}}`)
		return
	}
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	ctx := r.Context()
	containers, err := h.Docker.List(ctx)
	if err != nil {
		writeDockerError(w, r, err, "could not look up the app")
		return
	}
	g, found := findApp(containers, r.PathValue("id"))
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	root := g.root(h.StorageRoot)
	def, known := h.loadDefinition(root, g.appID)
	if _, curated := appcatalog.ByID(g.appID); curated {
		if body.Custom != nil || (body.AppID != "" && body.AppID != g.appID) {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "this is an app from the catalog")
			return
		}
		body.AppID = g.appID
	} else if body.Custom == nil || body.AppID != "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"custom": {...}}`)
		return
	}
	body.editID, body.Storage = g.appID, "" // an app keeps its id and its place
	keepSecrets(&body, def)

	rc, ferr := h.prepareRecreate(ctx, containers, g, root, body, def, known)
	if ferr != nil {
		ferr.write(w, r)
		return
	}
	pl := h.recreatePlan(ctx, rc, containers, recreateEdit)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}
