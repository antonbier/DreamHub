package api_test

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
)

type specOp struct {
	Parameters []struct {
		Ref  string `json:"$ref"`
		Name string `json:"name"`
	} `json:"parameters"`
	Box     string `json:"x-box"`
	Summary string `json:"summary"`
}

// specOps reads the served document into "METHOD /path" → operation, with
// the server prefix put back so it compares with the router's patterns.
func (e *env) specOps(t *testing.T) map[string]specOp {
	t.Helper()
	resp, b := e.do(t, "GET", "/api/v1/openapi.json", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi.json: %d %s", resp.StatusCode, b)
	}
	var doc struct {
		OpenAPI string                       `json:"openapi"`
		Paths   map[string]map[string]specOp `json:"paths"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("openapi.json is not valid JSON: %v", err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("openapi version %q", doc.OpenAPI)
	}
	ops := map[string]specOp{}
	for p, methods := range doc.Paths {
		for m, op := range methods {
			ops[strings.ToUpper(m)+" /api/v1"+p] = op
		}
	}
	return ops
}

// Routes outside the permission table (they are not in env.routes) that the
// document describes, and routes it leaves out on purpose.
var (
	publicSpecRoutes = []string{
		"GET /api/v1/health", "GET /api/v1/qr.png", "GET /api/v1/profiles",
		"POST /api/v1/profiles/{id}/session", "DELETE /api/v1/profiles/session",
		"GET /api/v1/peer/info", "POST /api/v1/peer/pair",
	}
	// A WebSocket proxy, not an API call.
	notInSpec = []string{"/api/v1/admin/terminal/proxy/"}
)

// The document and the router never drift apart: a route missing from the
// document fails here, and so does an operation in the document that no
// longer exists.
func TestOpenAPI_ParityWithTheRouter(t *testing.T) {
	e := newEnv(t)
	ops := e.specOps(t)

	have := map[string]bool{}
	for _, p := range publicSpecRoutes {
		have[p] = true
	}
	var missing []string
	for _, r := range e.routes {
		have[r.Pattern] = true
		if contains(notInSpec, r.Pattern) {
			continue
		}
		if _, ok := ops[r.Pattern]; !ok {
			missing = append(missing, r.Pattern)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("routes missing from internal/api/openapi.json:\n  %s", strings.Join(missing, "\n  "))
	}
	for op := range ops {
		if !have[op] {
			t.Errorf("openapi.json describes %s, which the router does not serve", op)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Every operation says whether the box matters: it takes the shared `box`
// parameter, or it is marked global. And every one that takes it really
// resolves a box — an unknown ID is refused before the handler runs.
func TestOpenAPI_EveryOperationResolvesABox(t *testing.T) {
	e := newEnv(t)
	for key, op := range e.specOps(t) {
		hasBox := false
		for _, p := range op.Parameters {
			if p.Ref == "#/components/parameters/Box" {
				hasBox = true
			}
		}
		if hasBox == (op.Box == "global") {
			t.Errorf("%s must either take the box parameter or be marked x-box: global", key)
			continue
		}
		if !hasBox || op.Summary == "" {
			if op.Summary == "" {
				t.Errorf("%s has no summary", key)
			}
			continue
		}
		method, path, _ := strings.Cut(key, " ")
		for _, seg := range []string{"{id}", "{key}", "{name}", "{size}", "{file}", "{action}"} {
			path = strings.ReplaceAll(path, seg, "x")
		}
		body := ""
		if method != "GET" {
			body = `{}`
		}
		resp, b := e.do(t, method, path+"?box=no-such-box", body, nil)
		if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(b), "box_not_found") {
			t.Errorf("%s with an unknown box: %d %s", key, resp.StatusCode, b)
		}
	}
}

// GET /boxes is what an external client starts from.
func TestOpenAPI_BoxesListCarriesCapabilities(t *testing.T) {
	e := newEnv(t)
	var out struct {
		Boxes []struct {
			ID           string   `json:"id"`
			Capabilities []string `json:"capabilities"`
		} `json:"boxes"`
		Default string `json:"default"`
	}
	e.json(t, "GET", "/api/v1/boxes", "", nil, 200, &out)
	if len(out.Boxes) == 0 || out.Default != out.Boxes[0].ID {
		t.Fatalf("boxes: %+v", out)
	}
	if !contains(out.Boxes[0].Capabilities, "webcontrol") {
		t.Errorf("capabilities of the first box: %v", out.Boxes[0].Capabilities)
	}
}
