// Package peer is how one DreamHub talks to the DreamHub on another box
// (docs/MULTIDEVICE_PLAN.md, M6): the one-time pairing code that proves
// someone stands at that box, the token it yields, the version handshake and
// the "what can you do here" question. Both ends live in this package: the
// Service answers (the DreamHub on the box being paired), the Client asks
// (the hub).
package peer

import (
	"context"
	"errors"
)

// APILevel is the level of the peer API this build speaks; MinPeerAPI is the
// lowest level of the other end this build can work with. Raise APILevel
// when the peer API changes, MinPeerAPI only when old ends stop working.
const (
	APILevel   = 1
	MinPeerAPI = 1
)

// Errors of pairing and asking a peer.
var (
	// ErrUnreachable: nothing answers at the address.
	ErrUnreachable = errors.New("peer: no answer")
	// ErrNoPeer: something answers but it is not a DreamHub that can be
	// paired (an older version, or the box's web interface).
	ErrNoPeer = errors.New("peer: that is not a DreamHub that can be paired")
	// ErrNotPairing: the DreamHub there shows no pairing code right now.
	ErrNotPairing = errors.New("peer: no pairing code is shown there")
	// ErrBadCode: the code is wrong or has expired.
	ErrBadCode = errors.New("peer: wrong or expired code")
	// ErrUnauthorized: the token is no longer valid there.
	ErrUnauthorized = errors.New("peer: the token was revoked")
	// ErrIncompatible: the versions can't work together; see Compat.
	ErrIncompatible = errors.New("peer: the versions do not fit")
)

// Info is what a DreamHub says about itself without a login — enough for the
// handshake and nothing a stranger on the LAN could misuse.
type Info struct {
	Version string `json:"version"`
	// API is the peer API level it speaks, MinHubAPI the lowest level of a
	// hub it can serve.
	API       int `json:"api"`
	MinHubAPI int `json:"minHubApi"`
	// DeviceName is the box's model name, to recognise it while pairing.
	DeviceName string `json:"deviceName"`
	// Pairing: a code is being shown there right now.
	Pairing bool `json:"pairing"`
}

// Status of the version handshake.
const (
	CompatOK     = "ok"
	CompatBoxOld = "box_too_old" // the DreamHub on the box needs an update
	CompatHubOld = "hub_too_old" // this DreamHub needs an update
)

// Compat decides from the other end's Info whether the two can work together.
func Compat(i Info) string {
	switch {
	case i.API < MinPeerAPI:
		return CompatBoxOld
	case APILevel < i.MinHubAPI:
		return CompatHubOld
	}
	return CompatOK
}

// Capabilities is the answer to "what can you do on your own machine".
type Capabilities struct {
	Info
	Capabilities []string `json:"capabilities"`
}

// PairResult is what a successful pairing yields.
type PairResult struct {
	// Token is the bearer token for later requests; shown once.
	Token string `json:"token"`
	Info  Info   `json:"info"`
}

// Client is the asking end (the hub), talking to the DreamHub at baseURL.
type Client interface {
	Info(ctx context.Context, baseURL string) (Info, error)
	Pair(ctx context.Context, baseURL, code, hubName string) (PairResult, error)
	Capabilities(ctx context.Context, baseURL, token string) (Capabilities, error)
}
