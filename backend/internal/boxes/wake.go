package boxes

import (
	"context"
	"errors"
	"net"
	"strings"
)

// Power-on methods of a box (docs/MULTIDEVICE_PLAN.md, M9).
const (
	WakeNone      = ""
	WakeWOL       = "wol"
	WakeExtension = "extension"
)

// Errors of switching a box on.
var (
	ErrNoWake   = errors.New("boxes: no way to switch this box on is set up")
	ErrWakeSend = errors.New("boxes: the wake-up packet could not be sent")
)

// ParseMAC accepts aa:bb:cc:dd:ee:ff, aa-bb-…, aabb.ccdd.eeff or the bare
// twelve hex digits, and returns it as aa:bb:cc:dd:ee:ff; "" if it isn't one.
func ParseMAC(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.ContainsAny(raw, ":-.") {
		var b strings.Builder
		for i := 0; i+1 < len(raw); i += 2 {
			if i > 0 {
				b.WriteByte(':')
			}
			b.WriteString(raw[i : i+2])
		}
		if len(raw) == 12 {
			raw = b.String()
		}
	}
	hw, err := net.ParseMAC(raw)
	if err != nil || len(hw) != 6 {
		return ""
	}
	return hw.String()
}

// MagicPacket is the Wake-on-LAN payload: six 0xFF bytes and the MAC
// sixteen times.
func MagicPacket(mac string) ([]byte, error) {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 {
		return nil, ErrWakeSend
	}
	p := make([]byte, 0, 102)
	for i := 0; i < 6; i++ {
		p = append(p, 0xFF)
	}
	for i := 0; i < 16; i++ {
		p = append(p, hw...)
	}
	return p, nil
}

// SendMagicPacket broadcasts the magic packet for mac on the local network
// (UDP port 9, the limited broadcast address). The box must have
// Wake-on-LAN switched on; nothing answers, so success only means "sent".
func SendMagicPacket(mac string) error {
	p, err := MagicPacket(mac)
	if err != nil {
		return err
	}
	c, err := net.Dial("udp4", "255.255.255.255:9")
	if err != nil {
		return ErrWakeSend
	}
	defer c.Close()
	if _, err := c.Write(p); err != nil {
		return ErrWakeSend
	}
	return nil
}

// Wake sends the Wake-on-LAN packet of a box set up for it. A box set up
// with an extension is switched on through the plan the API builds (the
// extension's own preview, since a socket may toggle) — not here.
func (m *Manager) Wake(ctx context.Context, id string) error {
	e, ok := m.Registry.Get(id)
	if !ok {
		return ErrUnknownBox
	}
	if e.WakeMethod != WakeWOL || e.WakeMAC == "" {
		return ErrNoWake
	}
	send := m.SendWOL
	if send == nil {
		send = SendMagicPacket
	}
	if err := send(e.WakeMAC); err != nil {
		return err
	}
	if m.OnWake != nil {
		m.OnWake(id)
	}
	return nil
}
