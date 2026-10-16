package hubs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Stream is StreamHub: live TV and recordings in the browser. The box
// converts nothing — it has no usable encoder (ARCHITECTURE.md, "Hardware
// encoder"). DreamHub passes the box's MPEG-TS through unchanged, and the
// browser decodes it (frontend/src/streaming, spikes/streamhub-player):
// FFmpeg in WebAssembly for MPEG-2 video and MP2/AC3/AAC audio, WebCodecs
// for H.264.
//
// Watching live needs a tuner: enigma2's stream proxy takes one that's
// free or already on the channel's transponder, and never switches the TV
// for it. With the reference box's single tuner, a channel on another
// transponder can't be streamed while the TV or a recording uses the
// tuner — the stream then stays empty, and the answer says so.
type Stream struct {
	WC webcontrol.Client
	// FS reads recordings in place when DreamHub runs on the box; else
	// they come through WebControl's /file.
	FS recfs.FS
	// PiconDirs: as Guide's; nil means DefaultPiconDirs.
	PiconDirs []string
	// FirstData is how long a live stream may stay empty before it counts
	// as "no tuner"; 0 means 12 s.
	FirstData time.Duration

	// slots limits the live streams per box: each box has its own tuners
	// and its own uplink.
	slotsMu sync.Mutex
	slots   map[string]chan struct{}
}

// maxLiveStreams: each stream is 3–15 Mbit/s out of the box, and every
// extra one may need a tuner of its own.
const maxLiveStreams = 2

// Error codes.
const (
	CodeStreamNoTuner = "stream_no_tuner" // the box sends nothing: no tuner free for this channel
	CodeStreamBusy    = "stream_busy"     // maxLiveStreams already running
)

// Mount registers StreamHub's routes.
func (h *Stream) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/stream/bouquets", h.bouquets)
	mux.HandleFunc("GET /api/v1/stream/channels", h.channels)
	mux.HandleFunc("GET /api/v1/stream/picon", h.picon)
	mux.HandleFunc("GET /api/v1/stream/live", h.live)
	mux.HandleFunc("GET /api/v1/stream/recording", h.recording)
}

func (h *Stream) bouquets(w http.ResponseWriter, r *http.Request) {
	list, err := h.WC.Bouquets(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	out := []webcontrol.Service{}
	for _, b := range list {
		if !webcontrol.IsMarker(b.ServiceReference) {
			out = append(out, b)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"bouquets": visibleBouquets(r.Context(), out)})
}

// channels: a bouquet's channels in its order with now/next, and what the
// box shows — "same as the TV" needs no tuner of its own.
func (h *Stream) channels(w http.ResponseWriter, r *http.Request) {
	bouquet := r.URL.Query().Get("bouquet")
	if bouquet == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "bouquet is required")
		return
	}
	ctx := r.Context()
	services, err := h.WC.Services(ctx, bouquet)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	var nowNext []webcontrol.NowNext
	if hasServices(services) { // see Radio.stations: an empty bouquet breaks epgnownext
		nowNext, _ = h.WC.EPGNowNext(ctx, bouquet)
	}
	playing, _ := h.WC.CurrentServiceReference(ctx)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"channels": mergeStations(services, nowNext, false), "playing": playing})
}

func (h *Stream) picon(w http.ResponseWriter, r *http.Request) { servePicon(w, r, h.PiconDirs) }

// dvbRef is a broadcast service reference ("1:0:19:283D:3FB:1:C00000:0:0:0:")
// — never a file or a URL (4097:…), which the stream proxy would also open.
var dvbRef = regexp.MustCompile(`^1:0:[0-9A-Fa-f]{1,8}(:[0-9A-Fa-f]{1,8}){7}:(0:)?$`)

// ValidLiveRef reports whether ref may be streamed live.
func ValidLiveRef(ref string) bool { return dvbRef.MatchString(ref) }

// acquire takes a live-stream slot of the box in ctx; the returned func
// gives it back. ok is false when the box already streams maxLiveStreams.
func (h *Stream) acquire(ctx context.Context) (release func(), ok bool) {
	key := box.Key(ctx)
	h.slotsMu.Lock()
	if h.slots == nil {
		h.slots = map[string]chan struct{}{}
	}
	ch := h.slots[key]
	if ch == nil {
		ch = make(chan struct{}, maxLiveStreams)
		h.slots[key] = ch
	}
	h.slotsMu.Unlock()
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, true
	default:
		return nil, false
	}
}

// live passes the box's stream of a channel through. The first bytes are
// awaited before answering, so "no tuner free" is an error the screen can
// explain rather than a player waiting forever.
func (h *Stream) live(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if !ValidLiveRef(ref) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "ref must be a broadcast service reference")
		return
	}
	release, ok := h.acquire(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusTooManyRequests, CodeStreamBusy, "too many live streams at once")
		return
	}
	defer release()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	body, err := h.WC.LiveStream(ctx, ref)
	if err != nil {
		if errors.Is(err, webcontrol.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusServiceUnavailable, CodeStreamNoTuner, "the box doesn't stream this channel")
			return
		}
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach the box's stream")
		return
	}
	defer body.Close()

	wait := h.FirstData
	if wait == 0 {
		wait = 12 * time.Second
	}
	timer := time.AfterFunc(wait, cancel) // unblocks the first read
	buf := make([]byte, 64*1024)
	n, err := io.ReadAtLeast(body, buf, 188)
	if !timer.Stop() || err != nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, CodeStreamNoTuner, "no tuner is free for this channel")
		return
	}
	streamHeaders(w)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf[:n]); err != nil {
		return
	}
	copyFlushing(w, body, buf)
}

// recording plays a recording, with Range for seeking: from disk when
// DreamHub runs on the box, else through WebControl's /file.
func (h *Stream) recording(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roots, err := h.WC.Locations(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	want := r.URL.Query().Get("path")
	if opener, ok := h.FS.(recfs.Opener); ok && onBox(ctx, h.FS, roots) {
		p, err := recfs.CleanIn(h.FS, want, roots)
		if err != nil || !strings.HasSuffix(p, ".ts") {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such recording")
			return
		}
		fi, err := h.FS.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such recording")
			return
		}
		f, err := opener.Open(p)
		if err != nil {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such recording")
			return
		}
		defer f.Close()
		streamHeaders(w)
		http.ServeContent(w, r, "", fi.ModTime(), f) // Range, If-Range, HEAD
		return
	}

	// Off the box: only a .ts below one of the box's recording folders.
	p := path.Clean("/" + want)
	if !strings.HasSuffix(p, ".ts") || !underAny(p, roots) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such recording")
		return
	}
	file, err := h.WC.RecordingFile(ctx, p, r.Header.Get("Range"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such recording")
		return
	}
	defer file.Body.Close()
	streamHeaders(w)
	w.Header().Set("Accept-Ranges", "bytes")
	if file.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(file.ContentLength, 10))
	}
	if file.ContentRange != "" {
		w.Header().Set("Content-Range", file.ContentRange)
	}
	w.WriteHeader(file.Status)
	copyFlushing(w, file.Body, make([]byte, 64*1024))
}

func underAny(p string, roots []string) bool {
	for _, root := range roots {
		root = strings.TrimSuffix(path.Clean("/"+root), "/") + "/"
		if strings.HasPrefix(p, root) {
			return true
		}
	}
	return false
}

func streamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// copyFlushing copies src to w, flushing every chunk so the browser gets
// the stream as it comes, until either side stops.
func copyFlushing(w http.ResponseWriter, src io.Reader, buf []byte) {
	rc := http.NewResponseController(w)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			_ = rc.Flush()
		}
		if err != nil {
			return
		}
	}
}
