package api

import (
	_ "embed"
	"net/http"
)

// openAPISpec is the description of /api/v1 for external developers. It is
// kept by hand and held in step with the router by TestOpenAPI_* (every
// route must be in it, and every operation must say whether it depends on
// the box).
//
//go:embed openapi.json
var openAPISpec []byte

func handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(openAPISpec)
}
