package webcontrol

import (
	"context"
	"strconv"
	"strings"
)

// Signal is the reception of the tuner that shows the current channel, as
// /web/signal reports it (<e2frontendstatus>; ARCHITECTURE.md, "VU+ Solo2
// with VTi 15"). SNR and AGC are percentages, BER the bit error count.
// SNRdB is set only when the box gives a real dB reading: VTi 15 answers
// "99 dB" next to "99 %" — the percentage again, not a dB value. AGC is set
// only when the box reports a strength: a Dreambox One answers "0 %" for it
// throughout, with a good SNR (its tuner driver gives no signal power).
type Signal struct {
	SNR   int      `json:"snr"`
	AGC   *int     `json:"agc,omitempty"`
	BER   int64    `json:"ber"`
	SNRdB *float64 `json:"snrDb,omitempty"`
}

// maxSNRdB is the highest dB value taken as a real reading; satellite and
// cable reception lies well below it.
const maxSNRdB = 40

// TunerSignal reads /web/signal. ErrNotFound when the box has no such
// call; nil and no error when it answers without numbers (no tuner in use,
// standby).
func (c *HTTPClient) TunerSignal(ctx context.Context) (*Signal, error) {
	var res struct {
		SNRdB string `xml:"e2snrdb"`
		SNR   string `xml:"e2snr"`
		BER   string `xml:"e2ber"`
		AGC   string `xml:"e2acg"`
	}
	if err := c.getXML(ctx, "/web/signal", &res); err != nil {
		return nil, err
	}
	snr, okSNR := leadingNumber(res.SNR)
	agc, okAGC := leadingNumber(res.AGC)
	if !okSNR && !okAGC {
		return nil, nil
	}
	out := &Signal{SNR: int(snr)}
	if okAGC && (agc > 0 || snr == 0) {
		a := int(agc)
		out.AGC = &a
	}
	if ber, ok := leadingNumber(res.BER); ok {
		out.BER = int64(ber)
	}
	if db, ok := leadingNumber(res.SNRdB); ok && db < maxSNRdB {
		out.SNRdB = &db
	}
	return out, nil
}

// leadingNumber reads the number a value like "91 %", "12.50 dB" or "0"
// starts with.
func leadingNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && (s[end] >= '0' && s[end] <= '9' || s[end] == '.' || s[end] == ',' || s[end] == '-' && end == 0) {
		end++
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(s[:end], ",", "."), 64)
	return n, err == nil
}
