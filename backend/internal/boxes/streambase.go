package boxes

import (
	"context"
	"net"
	"net/url"
)

// SetStreamBase tells the registry how DreamHub's own machine reaches itself
// ("http://127.0.0.1:8080", "" when DreamHub is not on a box) and the port
// the other boxes use to reach it.
func (r *Registry) SetStreamBase(local, port string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.localBase, r.port = local, port
}

// DetectOwnAddr finds this machine's address on the network that reaches
// host — the address that box has to use to reach this DreamHub. A variable
// so tests can replace it.
var DetectOwnAddr = func(host string) string {
	conn, err := net.Dial("udp", net.JoinHostPort(host, "9")) // no packet is sent
	if err != nil {
		return ""
	}
	defer conn.Close()
	a, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	return a.IP.String()
}

// StreamBase is how the box ctx names can reach this DreamHub — what a film
// played on that box is fetched from. Empty when that box can't play from
// here: a Basic box has no DreamHub player plugin, and DreamHub off-box has
// no address of its own to give.
//
// The box's own machine uses the loopback; a paired box uses the setting of
// that box, else the address of this machine on the network that reaches it.
func (r *Registry) StreamBase(ctx context.Context) string {
	e, err := r.For(ctx)
	if err != nil {
		return ""
	}
	return r.baseOf(e)
}

// PlayBase is StreamBase for the box with this ID.
func (r *Registry) PlayBase(id string) string {
	e, ok := r.Get(id)
	if !ok {
		return ""
	}
	return r.baseOf(e)
}

// LookupHost resolves a box's host name; a variable so tests can replace it.
var LookupHost = net.LookupHost

// Admits reports whether a request from ip may be a box's own player
// fetching a film: the machine itself for the local box, else the box's
// address. A box with an explicit stream address (a proxy, a VPN) is
// admitted from anywhere — the ticket is its credential then.
func (r *Registry) Admits(id string, ip net.IP) bool {
	e, ok := r.Get(id)
	if !ok || ip == nil {
		return false
	}
	switch {
	case e.Local:
		return ip.IsLoopback()
	case e.StreamBase != "":
		return true
	}
	u, err := url.Parse(e.URL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	addrs, err := LookupHost(u.Hostname())
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if net.ParseIP(a).Equal(ip) {
			return true
		}
	}
	return false
}

func (r *Registry) baseOf(e Entry) string {
	r.mu.RLock()
	local, port := r.localBase, r.port
	r.mu.RUnlock()
	switch {
	case e.Local:
		return local
	case !e.Paired():
		return ""
	case e.StreamBase != "":
		return e.StreamBase
	case port == "":
		return ""
	}
	u, err := url.Parse(e.URL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	ip := DetectOwnAddr(u.Hostname())
	if ip == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(ip, port)
}
