package webcontrol

import "context"

// Service references for ChannelHub (docs/CHANNELHUB_PLAN.md): every
// channel the box knows and its providers. These follow Enigma2's own
// conventions (the same queries its channel list and OpenWebif use); unlike
// RadioAll they haven't been tried on the reference box yet — the
// ChannelHub plan lists that as a read-only check.

// tvTypes are the TV service types: SD (1, 22 = H.264), HD (17 = MPEG-2,
// 25 = H.264), UHD (31 = HEVC) and the NVOD/feed variants 134 and 195.
const tvTypes = `(type == 1) || (type == 17) || (type == 22) || (type == 25) || (type == 31) || (type == 134) || (type == 195)`

// TVAll lists every TV channel the box knows, by name.
const TVAll = `1:7:1:0:0:0:0:0:0:0:` + tvTypes + ` ORDER BY name`

// TVProviders lists the TV providers; each entry's reference, passed to
// Services, lists that provider's channels.
const TVProviders = `1:7:1:0:0:0:0:0:0:0:` + tvTypes + ` FROM PROVIDERS ORDER BY name`

// RadioProviders is TVProviders for radio.
const RadioProviders = `1:7:2:0:0:0:0:0:0:0:(type == 2) || (type == 10) FROM PROVIDERS ORDER BY name`

// Quality is a channel's picture class from its service type: "uhd",
// "hd", "sd", or "" for radio and anything else.
func Quality(ref string) string {
	f, ok := serviceFields(ref)
	if !ok {
		return ""
	}
	switch f[2] {
	case 31:
		return "uhd"
	case 17, 25, 134, 195:
		return "hd"
	case 1, 22:
		return "sd"
	}
	return ""
}

// BouquetEditorProber is implemented by clients that can tell whether the
// optional BouquetEditor plugin (/bouqueteditor) is installed — ChannelHub
// edits only through it.
type BouquetEditorProber interface {
	BouquetEditorAvailable(ctx context.Context) bool
}
