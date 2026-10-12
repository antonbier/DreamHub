package main

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"time"
)

// eWeLink devices announce themselves over mDNS as _ewelink._tcp.local,
// their state (encrypted with the device key when "LAN control" is on)
// in the TXT record: id, type, seq, iv, encrypt, data1…data4. Only what
// that needs is parsed here — a hand-rolled reader of DNS messages
// (RFC 1035, with name compression), so the extension needs nothing but
// Go's standard library.

const serviceName = "_ewelink._tcp.local"

// Announcement is one device's mDNS answer.
type Announcement struct {
	Instance string            // e.g. "eWeLink_1002a17548._ewelink._tcp.local"
	IP       net.IP            // its A record
	Port     uint16            // its SRV port (8081 on the devices seen)
	TXT      map[string]string // id, type, seq, iv, encrypt, data1…
}

const (
	typeA   = 1
	typePTR = 12
	typeTXT = 16
	typeSRV = 33
)

// Browse asks the LAN for eWeLink devices and collects the answers that
// arrive within wait, one Announcement per device id.
func Browse(ctx context.Context, wait time.Duration) ([]Announcement, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	dst := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	query := buildQuery(serviceName, typePTR)
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	found := map[string]Announcement{}
	lastSend := time.Time{}
	buf := make([]byte, 9000)
	for time.Now().Before(deadline) {
		if time.Since(lastSend) > 700*time.Millisecond {
			if _, err := conn.WriteToUDP(query, dst); err != nil {
				return nil, err
			}
			lastSend = time.Now()
			_ = conn.SetReadDeadline(minTime(deadline, lastSend.Add(700*time.Millisecond)))
		}
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return nil, err
		}
		for _, a := range ParseResponse(buf[:n]) {
			if a.IP == nil {
				a.IP = from.IP
			}
			if id := a.TXT["id"]; id != "" {
				found[id] = a
			}
		}
	}
	out := make([]Announcement, 0, len(found))
	for _, a := range found {
		out = append(out, a)
	}
	return out, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func buildQuery(name string, qtype uint16) []byte {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[4:], 1) // one question
	for _, label := range strings.Split(name, ".") {
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	msg = append(msg, 0)
	return binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(msg, qtype), 1)
}

// ParseResponse reads every eWeLink instance out of one mDNS message.
// A malformed message yields what could be read before the damage.
func ParseResponse(msg []byte) []Announcement {
	if len(msg) < 12 {
		return nil
	}
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	total := int(binary.BigEndian.Uint16(msg[6:])) + int(binary.BigEndian.Uint16(msg[8:])) + int(binary.BigEndian.Uint16(msg[10:]))
	off := 12
	for i := 0; i < qd; i++ {
		_, n, ok := readName(msg, off)
		if !ok || n+4 > len(msg) {
			return nil
		}
		off = n + 4
	}
	byInstance := map[string]*Announcement{}
	hosts := map[string]net.IP{}
	srvHost := map[string]string{}
	get := func(name string) *Announcement {
		if a, ok := byInstance[name]; ok {
			return a
		}
		a := &Announcement{Instance: name, TXT: map[string]string{}}
		byInstance[name] = a
		return a
	}
	for i := 0; i < total; i++ {
		name, n, ok := readName(msg, off)
		if !ok || n+10 > len(msg) {
			break
		}
		rtype := binary.BigEndian.Uint16(msg[n:])
		rdlen := int(binary.BigEndian.Uint16(msg[n+8:]))
		rd := n + 10
		if rd+rdlen > len(msg) {
			break
		}
		data := msg[rd : rd+rdlen]
		switch rtype {
		case typePTR:
			if strings.EqualFold(name, serviceName) {
				if inst, _, ok := readName(msg, rd); ok {
					get(inst)
				}
			}
		case typeTXT:
			if strings.HasSuffix(strings.ToLower(name), "."+serviceName) {
				a := get(name)
				for p := 0; p < len(data); {
					l := int(data[p])
					if p+1+l > len(data) {
						break
					}
					if k, v, ok := strings.Cut(string(data[p+1:p+1+l]), "="); ok {
						a.TXT[k] = v
					}
					p += 1 + l
				}
			}
		case typeSRV:
			if len(data) >= 6 {
				a := get(name)
				a.Port = binary.BigEndian.Uint16(data[4:])
				if target, _, ok := readName(msg, rd+6); ok {
					srvHost[name] = target
				}
			}
		case typeA:
			if len(data) == 4 {
				hosts[strings.ToLower(name)] = net.IP(append([]byte(nil), data...))
			}
		}
		off = rd + rdlen
	}
	var out []Announcement
	for name, a := range byInstance {
		if ip, ok := hosts[strings.ToLower(srvHost[name])]; ok {
			a.IP = ip
		}
		if len(a.TXT) > 0 {
			out = append(out, *a)
		}
	}
	return out
}

// readName reads a (possibly compressed) domain name at off and returns
// it with the offset just past it in the original position.
func readName(msg []byte, off int) (string, int, bool) {
	var labels []string
	end := -1
	for jumps := 0; jumps < 20; {
		if off >= len(msg) {
			return "", 0, false
		}
		l := int(msg[off])
		switch {
		case l == 0:
			if end < 0 {
				end = off + 1
			}
			return strings.Join(labels, "."), end, true
		case l&0xC0 == 0xC0:
			if off+1 >= len(msg) {
				return "", 0, false
			}
			if end < 0 {
				end = off + 2
			}
			off = int(binary.BigEndian.Uint16(msg[off:]) & 0x3FFF)
			jumps++
		default:
			if off+1+l > len(msg) {
				return "", 0, false
			}
			labels = append(labels, string(msg[off+1:off+1+l]))
			off += 1 + l
		}
	}
	return "", 0, false
}
