package httpx

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RequestIDHeader carries the request ID in every response.
const RequestIDHeader = "X-Request-ID"

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush makes statusRecorder itself an http.Flusher when the ResponseWriter
// it wraps is one — needed for the SSE stream (GET /api/v1/events/stream):
// without this, wrapping loses Flush even though the real underlying
// writer supports it, since http.ResponseWriter itself doesn't declare it.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack makes statusRecorder itself an http.Hijacker when the
// ResponseWriter it wraps is one — the same reason Flush exists above,
// needed for AppsHub's console (GET /api/v1/apps/installed/{id}/console,
// internal/wsutil.Upgrade): every request already passes through this
// wrapper (RequestIDAndLogging), so without this the type assertion
// inside Upgrade would always fail regardless of the real server.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("httpx: underlying ResponseWriter doesn't support Hijack")
	}
	return hj.Hijack()
}

// RequestIDAndLogging gives every request a fresh ID (never trusting one
// sent by the client), returns it in the X-Request-ID header, stores it in
// the context and logs one line per request with it.
func RequestIDAndLogging(logger *slog.Logger, now func() time.Time, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set(RequestIDHeader, id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := now()
		next.ServeHTTP(rec, r.WithContext(WithRequestID(r.Context(), id)))
		logger.Info("request",
			"requestId", id, "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "durationMs", now().Sub(start).Milliseconds())
	})
}

// CSRFHeader must be present on every mutating request. Browsers can't add
// custom headers to cross-origin requests without a CORS preflight, and
// DreamHub never answers preflights with CORS headers — so a foreign web page
// can't forge a mutating request, even though the browser would attach Basic
// Auth credentials automatically (PRINCIPLES.md, "Security in the LAN").
const CSRFHeader = "X-DreamHub-Request"

func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// CSRF rejects mutating requests that lack CSRFHeader or come from a foreign
// Origin.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMutating(r.Method) {
			if r.Header.Get(CSRFHeader) == "" {
				WriteError(w, r, http.StatusForbidden, "csrf_failed", "mutating requests need the "+CSRFHeader+" header")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					WriteError(w, r, http.StatusForbidden, "csrf_failed", "cross-origin mutating requests are not allowed")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// HostAllowed decides whether a Host header is acceptable. DNS rebinding
// works by pointing a foreign domain name at a LAN address, so requests
// addressed by name are only accepted for known names: localhost, mDNS
// ".local" names, and an explicit allowlist. Requests addressed by IP (how
// the box is normally reached, including over ZeroTier) are always fine.
func HostAllowed(host string, allowed []string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.TrimSuffix(strings.ToLower(strings.Trim(h, "[]")), ".")
	if h == "" {
		return false
	}
	if net.ParseIP(h) != nil || h == "localhost" || strings.HasSuffix(h, ".local") {
		return true
	}
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimSpace(a), h) {
			return true
		}
	}
	return false
}

// HostCheck rejects requests whose Host header isn't allowed. allowed is
// called per request so allowlist changes in settings apply immediately.
func HostCheck(allowed func(*http.Request) []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !HostAllowed(r.Host, allowed(r)) {
			WriteError(w, r, http.StatusMisdirectedRequest, "host_not_allowed",
				"unknown host name; add it to network.allowedHosts in settings or use the box's IP address")
			return
		}
		next.ServeHTTP(w, r)
	})
}
