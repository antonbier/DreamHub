package wsutil

import (
	"bufio"
	"encoding/binary"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testClient is a bare-bones WebSocket client (masked client frames, no
// fragmentation) — just enough to exercise the server side for real
// over an actual TCP connection, the same "don't just assume the wire
// format" standard this repo's other hand-rolled protocol tests hold to
// (e.g. internal/dockerd's fakeDaemon).
type testClient struct {
	conn net.Conn
	br   *bufio.Reader
}

func dialTestClient(t *testing.T, addr string) *testClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
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
	return &testClient{conn: conn, br: br}
}

func (c *testClient) send(opcode int, payload []byte) {
	var mask [4]byte
	_, _ = rand.Read(mask[:])
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	head := []byte{0x80 | byte(opcode), 0x80 | byte(len(payload))}
	_, _ = c.conn.Write(head)
	_, _ = c.conn.Write(mask[:])
	_, _ = c.conn.Write(masked)
}

func (c *testClient) recv() (opcode int, payload []byte) {
	var head [2]byte
	_, _ = c.br.Read(head[:])
	op := int(head[0] & 0x0F)
	n := uint64(head[1] & 0x7F)
	if n == 126 {
		var ext [2]byte
		_, _ = c.br.Read(ext[:])
		n = uint64(binary.BigEndian.Uint16(ext[:]))
	}
	data := make([]byte, n)
	_, _ = c.br.Read(data)
	return op, data
}

func TestUpgrade_EchoRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		op, data, err := conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.WriteMessage(op, data)
	}))
	defer srv.Close()

	c := dialTestClient(t, srv.Listener.Addr().String())
	defer c.conn.Close()

	c.send(OpBinary, []byte("hello"))
	op, data := c.recv()
	if op != OpBinary || string(data) != "hello" {
		t.Fatalf("got opcode %d, payload %q", op, data)
	}
}

func TestUpgrade_RejectsPlainRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := Upgrade(w, r); err == nil {
			t.Error("want an error for a non-websocket request")
		}
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", resp.StatusCode)
	}
}
