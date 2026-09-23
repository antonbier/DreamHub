// Package httpx holds the HTTP conventions shared by the core API and every
// hub: the JSON error envelope (PRINCIPLES.md, "Consistent API error shape"),
// request IDs, the current user in the request context, and body decoding.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyUser
)

// WithRequestID stores the request ID in ctx.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestID returns the request ID stored in ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(keyRequestID).(string)
	return id
}

// WithUser stores the current user in ctx.
func WithUser(ctx context.Context, u store.User) context.Context {
	return context.WithValue(ctx, keyUser, u)
}

// User returns the current user stored in ctx.
func User(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(keyUser).(store.User)
	return u, ok
}

// ErrorBody is the content of the error envelope. Code is machine-readable
// and translated by clients; Message is an English hint for developers;
// RequestID links the error to the matching log entries.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
	Details   any    `json:"details,omitempty"`
}

// FieldError is a validation problem for one input field: a code plus
// parameters, never finished text.
type FieldError struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// WriteJSON writes a successful JSON response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the JSON error envelope.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	WriteErrorDetails(w, r, status, code, message, nil)
}

// WriteErrorDetails writes the JSON error envelope with details (e.g. a map
// of field → FieldError).
func WriteErrorDetails(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	body := ErrorBody{Code: code, Message: message, Details: details}
	if r != nil {
		body.RequestID = RequestID(r.Context())
	}
	WriteJSON(w, status, map[string]ErrorBody{"error": body})
}

// MaxBodyBytes caps request bodies; DreamHub's requests are small.
const MaxBodyBytes = 64 << 10

// ErrInvalidBody is returned by DecodeJSON for malformed input.
var ErrInvalidBody = errors.New("httpx: invalid request body")

// DecodeJSON decodes a size-limited JSON body into dst, rejecting unknown
// fields and trailing data.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return ErrInvalidBody
	}
	if dec.More() {
		return ErrInvalidBody
	}
	return nil
}
