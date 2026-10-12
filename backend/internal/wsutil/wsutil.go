// Package wsutil is a minimal, hand-rolled WebSocket server (RFC 6455) —
// the same house choice this repo already makes everywhere else (no SDK
// dependency for one small, legible protocol implementation instead of a
// large one this project doesn't need most of, see internal/dockerd's
// own doc comment). It exists for exactly one caller, AppsHub's
// per-container console (docs/APPSHUB_PLAN.md, App5): bridging a
// browser's WebSocket to a docker exec session's raw byte stream
// (internal/dockerd.Client.Attach) needs *some* framed, full-duplex
// browser-native transport, and nothing else in this codebase talks
// WebSocket today.
//
// Deliberately narrow: single, unfragmented text/binary/close/ping/pong
// frames only (no continuation frames, no extensions, no
// compression) — everything this package's one caller ever sends is a
// single small chunk per message, so supporting fragmented messages
// would be weight with nothing to exercise it.
package wsutil

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Opcodes (RFC 6455 §5.2).
const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

// handshakeGUID is RFC 6455's own fixed magic string for computing
// Sec-WebSocket-Accept.
const handshakeGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// ErrClosed is returned by ReadMessage once a close frame (from either
// side) has ended the session.
var ErrClosed = errors.New("wsutil: connection closed")

// Conn is one upgraded WebSocket connection. Not safe for concurrent
// writes from multiple goroutines (this package's only caller writes
// from a single pump goroutine); reads are only ever done from one
// goroutine too.
type Conn struct {
	rw   io.ReadWriteCloser
	br   *bufio.Reader
	done bool
}

// Upgrade performs the RFC 6455 handshake over an already-hijacked (or
// hijackable) HTTP connection and returns a Conn ready for
// ReadMessage/WriteMessage. The caller's ResponseWriter must support
// http.Hijacker (true for the stdlib's plain HTTP/1.1 server, which is
// all this process ever runs behind).
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "wsutil: not a websocket upgrade request", http.StatusBadRequest)
		return nil, errors.New("wsutil: missing or wrong Upgrade/Sec-WebSocket-Key header")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "wsutil: hijack unsupported", http.StatusInternalServerError)
		return nil, errors.New("wsutil: ResponseWriter doesn't support Hijack")
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	accept := acceptKey(key)
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := conn.Write([]byte(resp)); err != nil {
		conn.Close()
		return nil, err
	}
	// buf may already hold bytes read past the request's own headers
	// (shouldn't, for a GET with no body, but a buffered reader is kept
	// regardless rather than assumed empty).
	return &Conn{rw: conn, br: buf.Reader}, nil
}

func acceptKey(clientKey string) string {
	h := sha1.New()
	h.Write([]byte(clientKey + handshakeGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// ReadMessage reads one complete, unfragmented frame and returns its
// opcode and payload. A close frame returns (OpClose, payload, nil)
// once; ErrClosed on every call after.
func (c *Conn) ReadMessage() (opcode int, payload []byte, err error) {
	if c.done {
		return 0, nil, ErrClosed
	}
	var head [2]byte
	if _, err := io.ReadFull(c.br, head[:]); err != nil {
		c.done = true
		return 0, nil, err
	}
	fin := head[0]&0x80 != 0
	op := int(head[0] & 0x0F)
	masked := head[1]&0x80 != 0
	length := uint64(head[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			c.done = true
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			c.done = true
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if !fin {
		c.done = true
		return 0, nil, errors.New("wsutil: fragmented messages aren't supported")
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, maskKey[:]); err != nil {
			c.done = true
			return 0, nil, err
		}
	}
	data := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(c.br, data); err != nil {
			c.done = true
			return 0, nil, err
		}
	}
	if masked {
		for i := range data {
			data[i] ^= maskKey[i%4]
		}
	}
	if op == OpClose {
		c.done = true
		_ = c.writeFrame(OpClose, nil)
		return OpClose, data, nil
	}
	if op == OpPing {
		_ = c.writeFrame(OpPong, data)
		return c.ReadMessage() // a ping isn't a message this caller cares about; keep reading
	}
	return op, data, nil
}

// WriteMessage writes one complete, unfragmented frame. The server
// never masks its own frames (RFC 6455 §5.1 — only client-to-server
// frames are masked).
func (c *Conn) WriteMessage(opcode int, payload []byte) error {
	if c.done {
		return ErrClosed
	}
	return c.writeFrame(opcode, payload)
}

func (c *Conn) writeFrame(opcode int, payload []byte) error {
	var head []byte
	n := len(payload)
	switch {
	case n <= 125:
		head = []byte{0x80 | byte(opcode), byte(n)}
	case n <= 0xFFFF:
		head = make([]byte, 4)
		head[0] = 0x80 | byte(opcode)
		head[1] = 126
		binary.BigEndian.PutUint16(head[2:], uint16(n))
	default:
		head = make([]byte, 10)
		head[0] = 0x80 | byte(opcode)
		head[1] = 127
		binary.BigEndian.PutUint64(head[2:], uint64(n))
	}
	if _, err := c.rw.Write(head); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.rw.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the underlying connection without a close handshake —
// used when the bridge loop is tearing down anyway (the other side,
// docker exec, gets its own Close from dockerd.Client.Attach's own
// ReadWriteCloser).
func (c *Conn) Close() error {
	c.done = true
	return c.rw.Close()
}
