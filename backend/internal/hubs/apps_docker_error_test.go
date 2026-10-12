package hubs

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
)

// On a box without Docker the helper's address refuses the connection;
// that must read as docker_unavailable (the Apps tab keeps the catalog
// and says Docker isn't set up), not a generic docker_failed. Found on
// the reference box, 2026-10-12, which has no Docker installed.
func TestWriteDockerError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
		want string
	}{
		{fmt.Errorf("%w: dial tcp 127.0.0.1:8090: connection refused", dockerd.ErrUnavailable), http.StatusServiceUnavailable, "docker_unavailable"},
		{errors.New("daemon said 500"), http.StatusBadGateway, "docker_failed"},
	} {
		rec := httptest.NewRecorder()
		writeDockerError(rec, httptest.NewRequest("GET", "/", nil), tc.err, "x")
		if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%v: got %d %s", tc.err, rec.Code, rec.Body.String())
		}
	}
}
