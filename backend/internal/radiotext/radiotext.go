// Package radiotext reads the radio text (the "now playing" line) of a
// radio station out of its MPEG-TS stream (ROADMAP.md, RadioHub, #28).
//
// WebControl doesn't expose it — only Enigma2's own GUI decodes it — but
// it is in the audio stream: DVB radio carries RDS in the ancillary data
// at the end of each MPEG-1 Layer II audio frame (confirmed on the
// reference box, 2026-10-20, with Dlf Kultur: the frames end with the
// marker byte 0xFD, the byte before is the number of RDS bytes, and those
// bytes sit in reverse order). Strung together they form UECP messages
// (FE … FF, with 0xFD as the escape); message element 0x0A is the radio
// text.
package radiotext

import (
	"io"
	"strings"
)

const tsPacket = 188

// Read follows the TS stream r until a complete radio text has been seen
// (it returns the last one) or r ends or max bytes have been read. The
// text is "" when the station sends none.
func Read(r io.Reader, max int) (string, error) {
	var (
		d       demux
		buf     = make([]byte, 64*1024)
		pending []byte
		total   int
		best    string
		data    []byte // RDS bytes, in stream order
	)
	for total < max {
		n, err := r.Read(buf)
		total += n
		pending = append(pending, buf[:n]...)
		pending = d.feed(pending, func(es []byte) {
			data = append(data, ancillary(&d, es)...)
		})
		if text := lastText(data); text != "" {
			best = text
			// One complete message is enough, but the one on air may
			// change within seconds: stop at the first that is complete.
			return best, nil
		}
		if err != nil {
			if err == io.EOF {
				return best, nil
			}
			return best, err
		}
	}
	return best, nil
}

// demux picks the audio elementary stream out of a transport stream: the
// first PID whose PES packets carry an MPEG audio stream ID.
type demux struct {
	audioPID int // 0: not found yet
	es       []byte
}

// feed consumes whole TS packets of b and gives the audio payload to out;
// it returns what is left (a partial packet).
func (d *demux) feed(b []byte, out func(es []byte)) []byte {
	// Find the sync byte, then take packets while it repeats.
	for len(b) >= tsPacket {
		if b[0] != 0x47 {
			b = b[1:]
			continue
		}
		p := b[:tsPacket]
		b = b[tsPacket:]
		pid := int(p[1]&0x1F)<<8 | int(p[2])
		if p[3]&0x10 == 0 { // no payload
			continue
		}
		start := 4
		if p[3]&0x20 != 0 { // adaptation field
			start += 1 + int(p[4])
		}
		if start >= tsPacket {
			continue
		}
		payload := p[start:]
		pusi := p[1]&0x40 != 0
		if d.audioPID == 0 && pusi && len(payload) >= 9 && payload[0] == 0 && payload[1] == 0 && payload[2] == 1 && payload[3] >= 0xC0 && payload[3] <= 0xDF {
			d.audioPID = pid
		}
		if pid != d.audioPID || d.audioPID == 0 {
			continue
		}
		if pusi {
			if len(payload) < 9 || payload[0] != 0 || payload[1] != 0 || payload[2] != 1 {
				continue
			}
			hdr := 9 + int(payload[8])
			if hdr > len(payload) {
				continue
			}
			payload = payload[hdr:]
		}
		out(payload)
	}
	return b
}

// MPEG-1 Layer II tables.
var (
	bitrates = [...]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384}
	rates    = [...]int{44100, 48000, 32000}
)

// ancillary collects the RDS bytes of the audio frames that are complete
// in the elementary stream so far (kept in d.es).
func ancillary(d *demux, es []byte) []byte {
	d.es = append(d.es, es...)
	var out []byte
	i := 0
	for i+4 <= len(d.es) {
		b := d.es
		if b[i] != 0xFF || b[i+1]&0xFE != 0xFC { // MPEG-1 Layer II
			i++
			continue
		}
		br, sr := int(b[i+2]>>4), int(b[i+2]>>2)&3
		if br == 0 || br == 15 || sr == 3 {
			i++
			continue
		}
		n := 144*bitrates[br]*1000/rates[sr] + int(b[i+2]>>1)&1
		if i+n > len(b) {
			break // the frame isn't all here yet
		}
		frame := b[i : i+n]
		i += n
		if frame[n-1] != 0xFD {
			continue
		}
		l := int(frame[n-2])
		if l == 0 || l > n-6 {
			continue
		}
		raw := frame[n-2-l : n-2]
		for k := len(raw) - 1; k >= 0; k-- { // stored in reverse
			out = append(out, raw[k])
		}
	}
	d.es = append(d.es[:0], d.es[i:]...)
	return out
}

// lastText finds the last complete radio text message in the RDS bytes.
func lastText(data []byte) string {
	text := ""
	for i := 0; i < len(data); i++ {
		if data[i] != 0xFE {
			continue
		}
		// Message up to the stop byte, un-stuffing 0xFD escapes.
		var msg []byte
		j := i + 1
		done := false
		for ; j < len(data); j++ {
			c := data[j]
			if c == 0xFF {
				done = true
				break
			}
			if c == 0xFD && j+1 < len(data) {
				j++
				switch data[j] {
				case 0x00:
					c = 0xFD
				case 0x01:
					c = 0xFE
				case 0x02:
					c = 0xFF
				}
			}
			msg = append(msg, c)
		}
		if !done {
			break
		}
		if t, ok := parseMessage(msg); ok {
			text = t
		}
		i = j
	}
	return text
}

// parseMessage reads a UECP message (without its start and stop bytes):
// address (2), sequence counter, field length, then the element: 0x0A is
// radio text — data set and programme service number, the length of the
// data, a flags byte, the text.
func parseMessage(m []byte) (string, bool) {
	if len(m) < 8 || m[4] != 0x0A {
		return "", false
	}
	mel := int(m[7])
	if mel < 1 || 8+mel > len(m) {
		return "", false
	}
	return clean(m[9 : 8+mel]), true
}

// clean turns the text's bytes into a string: ISO 8859-1, line ends and
// padding as spaces, trimmed.
func clean(b []byte) string {
	var s strings.Builder
	for _, c := range b {
		switch {
		case c == 0x0D || c == 0x0A || c == 0x00 || c == 0x0B:
			s.WriteByte(' ')
		case c < 0x20:
		default:
			s.WriteRune(rune(c))
		}
	}
	return strings.Join(strings.Fields(s.String()), " ")
}
