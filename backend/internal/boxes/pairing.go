package boxes

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/peer"
)

// Errors of pairing a box (docs/MULTIDEVICE_PLAN.md, M6).
var (
	ErrIsLocal   = errors.New("boxes: DreamHub runs on that box itself")
	ErrNoPairing = errors.New("boxes: no way to pair with other boxes")
)

// CompatError: the DreamHub on the box and this one don't fit.
type CompatError struct {
	Status string // peer.CompatBoxOld or peer.CompatHubOld
	Info   peer.Info
}

func (e *CompatError) Error() string { return "boxes: versions do not fit: " + e.Status }

// DefaultPeerPort is where DreamHub listens on a box.
const DefaultPeerPort = "8080"

// PairInput is what pairing a box needs: the one-time code shown there, and
// optionally the DreamHub's address when it isn't the box's address on the
// default port.
type PairInput struct {
	Code string `json:"code"`
	URL  string `json:"url"`
}

// peerURL is the address of the DreamHub on a box: the given one, else the
// box's host on the default port.
func peerURL(b string, given string) string {
	if u := normalizeURL(given); u != "" {
		return u
	}
	u, err := url.Parse(b)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(u.Hostname(), DefaultPeerPort)
}

// Pair pairs with the DreamHub on a box: it checks that one answers and that
// the versions fit, redeems the code and keeps the token. hubName is how this
// DreamHub names itself in the other one's token list.
func (m *Manager) Pair(ctx context.Context, id string, in PairInput, hubName string) (Entry, peer.Info, error) {
	if m.Peer == nil {
		return Entry{}, peer.Info{}, ErrNoPairing
	}
	e, ok := m.Registry.Get(id)
	if !ok {
		return Entry{}, peer.Info{}, ErrUnknownBox
	}
	if e.Local {
		return Entry{}, peer.Info{}, ErrIsLocal
	}
	base := peerURL(e.URL, in.URL)
	if base == "" {
		return Entry{}, peer.Info{}, &ValidationError{Fields: map[string]string{"url": "invalid"}}
	}
	if strings.TrimSpace(in.Code) == "" {
		return Entry{}, peer.Info{}, &ValidationError{Fields: map[string]string{"code": "required"}}
	}
	info, err := m.Peer.Info(ctx, base)
	if err != nil {
		return Entry{}, peer.Info{}, err
	}
	if st := peer.Compat(info); st != peer.CompatOK {
		return Entry{}, info, &CompatError{Status: st, Info: info}
	}
	res, err := m.Peer.Pair(ctx, base, in.Code, hubName)
	if err != nil {
		return Entry{}, info, err
	}
	b := e.Box
	b.PeerURL, b.PeerToken = base, res.Token
	if err := m.Store.SaveBox(ctx, b); err != nil {
		return Entry{}, info, err
	}
	m.Registry.Add(b, e.Clients, false)
	e, _ = m.Registry.Get(id)
	return e, res.Info, nil
}

// Unpair forgets the token. The token stays valid on the box until it is
// revoked in that DreamHub's Settings → Authentication.
func (m *Manager) Unpair(ctx context.Context, id string) (Entry, error) {
	e, ok := m.Registry.Get(id)
	if !ok {
		return Entry{}, ErrUnknownBox
	}
	b := e.Box
	b.PeerURL, b.PeerToken = "", ""
	if err := m.Store.SaveBox(ctx, b); err != nil {
		return Entry{}, err
	}
	m.Registry.Add(b, e.Clients, e.Local)
	e, _ = m.Registry.Get(id)
	return e, nil
}

// Peer states of a paired box.
const (
	PeerOK          = "ok"
	PeerUnreachable = "unreachable"
	PeerRevoked     = "revoked" // the other DreamHub no longer accepts the token
)

// PeerStatus is what asking the paired DreamHub yields.
type PeerStatus struct {
	State        string
	Info         peer.Info
	Compat       string
	Capabilities []string
}

// PeerStatus asks the DreamHub on a paired box about itself.
func (m *Manager) PeerStatus(ctx context.Context, e Entry) PeerStatus {
	if m.Peer == nil || !e.Paired() {
		return PeerStatus{}
	}
	c, err := m.Peer.Capabilities(ctx, e.PeerURL, e.PeerToken)
	switch {
	case errors.Is(err, peer.ErrUnauthorized):
		return PeerStatus{State: PeerRevoked}
	case err != nil:
		return PeerStatus{State: PeerUnreachable}
	}
	return PeerStatus{State: PeerOK, Info: c.Info, Compat: peer.Compat(c.Info), Capabilities: c.Capabilities}
}
