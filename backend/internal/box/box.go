// Package box carries the active box through a request (docs/MULTIDEVICE_PLAN.md,
// M1). It is deliberately tiny and dependency-free so the store, the plan
// service and the clients can all read it.
//
// A request names its box with the `box=<id>` query parameter; the API
// layer resolves it and puts the ID into the context. Everything below
// (the web-control router, per-box tables, caches) reads it from there. A
// context without a box means "the first box", so existing clients, API
// tokens and background work keep meaning what they always meant.
package box

import "context"

// DefaultID is the ID of the box that existed before multi-box support:
// the one-time migration assigns all earlier data to it.
const DefaultID = "main"

type ctxKey struct{}

// With returns ctx addressed to the box with this ID.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// ID is the box ctx is addressed to, or "" when it names none.
func ID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Key is ID with the unaddressed case mapped to DefaultID — what a table
// keyed by box stores.
func Key(ctx context.Context) string {
	if id := ID(ctx); id != "" {
		return id
	}
	return DefaultID
}

type remoteKey struct{}

// WithRemote marks ctx as addressed to a box that is not the machine
// DreamHub runs on: its recordings, services and packages are not
// reachable through this process's own file system or system tools.
func WithRemote(ctx context.Context) context.Context {
	return context.WithValue(ctx, remoteKey{}, true)
}

// Remote reports whether ctx is addressed to a box other than DreamHub's
// own machine. The hubs' local-only features (file operations, in-place
// recording reads, the box's own logs) check it.
func Remote(ctx context.Context) bool {
	r, _ := ctx.Value(remoteKey{}).(bool)
	return r
}
