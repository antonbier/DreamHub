package api_test

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"math/rand"
	"net"
	"net/http"
	"testing"
)

type consoleStatus struct {
	Available bool `json:"available"`
	AuthOn    bool `json:"authOn"`
	Enabled   bool `json:"enabled"`
}

func (e *env) consoleStatus(t *testing.T, configure func(*http.Request)) consoleStatus {
	t.Helper()
	_, b := e.do(t, "GET", "/api/v1/apps/console/status", "", configure)
	var s consoleStatus
	_ = json.Unmarshal(b, &s)
	return s
}

func TestAppsConsole_NeedsAuthEnabledFirst(t *testing.T) {
	e := newEnv(t)
	if s := e.consoleStatus(t, nil); !s.Available || s.AuthOn || s.Enabled {
		t.Fatalf("status: %+v", s)
	}
	resp, _ := e.do(t, "POST", "/api/v1/apps/console/enable", `{"password":"anything"}`, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 auth_required, got %d", resp.StatusCode)
	}
}

func TestAppsConsole_WrongPasswordRejected(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)
	auth := func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") }

	resp, _ := e.do(t, "POST", "/api/v1/apps/console/enable", `{"password":"wrong"}`, auth)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a wrong password, got %d", resp.StatusCode)
	}
	if s := e.consoleStatus(t, auth); !s.AuthOn || s.Enabled {
		t.Fatalf("status after a rejected enable: %+v", s)
	}
}

func TestAppsConsole_DisabledRefusesAttach(t *testing.T) {
	e := newEnv(t)
	resp, _ := e.do(t, "GET", "/api/v1/apps/installed/pihole/console", "", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 console_disabled, got %d", resp.StatusCode)
	}
}

func TestAppsConsole_EnableDisableAndAttach(t *testing.T) {
	e := newEnv(t)
	e.do(t, "PATCH", "/api/v1/settings", `{"values":{"auth.enabled":true,"auth.username":"armin","auth.password":"correct horse"}}`, nil)
	auth := func(r *http.Request) { r.SetBasicAuth("armin", "correct horse") }

	resp, _ := e.do(t, "POST", "/api/v1/apps/console/enable", `{"password":"correct horse"}`, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable: got %d", resp.StatusCode)
	}
	if s := e.consoleStatus(t, auth); !s.Enabled {
		t.Fatalf("status after enable: %+v", s)
	}

	// Attach for real: a raw WebSocket handshake + a round trip against
	// the mock PTY (internal/dockerd/mock), the same "don't just trust
	// the handler, dial it" standard this repo's other hand-rolled
	// protocols get (wsutil's own tests, dockerd's fakeDaemon).
	conn, br := dialWS(t, e.srv.Listener.Addr().String(), "/api/v1/apps/installed/pihole/console", "armin", "correct horse")
	defer conn.Close()
	op, data := wsRecv(t, br)
	if op != 2 /* binary */ || len(data) == 0 {
		t.Fatalf("got opcode %d, payload %q, want a non-empty binary greeting", op, data)
	}
	wsSend(conn, 2, []byte("hello"))
	op, data = wsRecv(t, br)
	if op != 2 || string(data) == "" {
		t.Fatalf("got opcode %d, payload %q after writing", op, data)
	}

	resp, _ = e.do(t, "POST", "/api/v1/apps/console/disable", "", auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable: got %d", resp.StatusCode)
	}
	if s := e.consoleStatus(t, auth); s.Enabled {
		t.Fatalf("status after disable: %+v", s)
	}
}

func TestAppsConsole_Logs(t *testing.T) {
	e := newEnv(t)
	resp, b := e.do(t, "GET", "/api/v1/apps/installed/pihole/logs", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(b, &out)
	if out.Text == "" {
		t.Fatal("got empty logs text")
	}

	resp, _ = e.do(t, "GET", "/api/v1/apps/installed/not-an-app/logs", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

// --- a tiny raw WebSocket client, only for these tests ----------------------

func dialWS(t *testing.T, addr, path, user, pass string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n"
	if user != "" {
		req += "Authorization: Basic " + basicAuthValue(user, pass) + "\r\n"
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("got status %d, want 101", resp.StatusCode)
	}
	return conn, br
}

func basicAuthValue(user, pass string) string {
	req, _ := http.NewRequest("GET", "http://x/", nil)
	req.SetBasicAuth(user, pass)
	return req.Header.Get("Authorization")[len("Basic "):]
}

func wsSend(conn net.Conn, opcode int, payload []byte) {
	var mask [4]byte
	_, _ = rand.Read(mask[:])
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	head := []byte{0x80 | byte(opcode), 0x80 | byte(len(payload))}
	_, _ = conn.Write(head)
	_, _ = conn.Write(mask[:])
	_, _ = conn.Write(masked)
}

func wsRecv(t *testing.T, br *bufio.Reader) (int, []byte) {
	t.Helper()
	var head [2]byte
	if _, err := br.Read(head[:]); err != nil {
		t.Fatal(err)
	}
	op := int(head[0] & 0x0F)
	n := uint64(head[1] & 0x7F)
	if n == 126 {
		var ext [2]byte
		_, _ = br.Read(ext[:])
		n = uint64(binary.BigEndian.Uint16(ext[:]))
	}
	data := make([]byte, n)
	if n > 0 {
		if _, err := br.Read(data); err != nil {
			t.Fatal(err)
		}
	}
	return op, data
}
