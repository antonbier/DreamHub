package hubs

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/logosearch"
)

// A logo for one channel, chosen by hand (ChannelHub → Logos, #51): from
// any installed set, an uploaded picture, or the public TV-logo
// collection. It's DreamHub's own choice — a PNG kept in DreamHub's data
// folder and looked at before every picon set — so nothing on the box or
// the TV changes, and "reset" just deletes the file.

var (
	logoDirMu sync.Mutex
	logoDir   string
)

// SetLogoDir is where hand-chosen logos are kept; "" turns them off.
func SetLogoDir(dir string) {
	logoDirMu.Lock()
	logoDir = dir
	logoDirMu.Unlock()
}

func logoDirectory() string {
	logoDirMu.Lock()
	defer logoDirMu.Unlock()
	return logoDir
}

// logoKey is a service reference as a file name, the way picon files are
// named ("1_0_19_2EE3_441_1_C00000_0_0_0"); "" if ref isn't one.
func logoKey(ref string) string {
	m := piconRef.FindString(ref)
	if m == "" {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(strings.TrimRight(m, ":"), ":", "_"))
}

// readLogoOverride returns the hand-chosen logo of ref, if there is one.
func readLogoOverride(ref string) ([]byte, bool) {
	dir, key := logoDirectory(), logoKey(ref)
	if dir == "" || key == "" {
		return nil, false
	}
	p := filepath.Join(dir, key+".png")
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil || http.DetectContentType(b) != "image/png" {
		return nil, false
	}
	return b, true
}

// Logo size limits: what an upload may be, and what is stored.
const (
	maxLogoUpload = 4 << 20
	maxLogoSide   = 4000
	storedWidth   = 400
	storedHeight  = 240
	maxStoredLogo = 1 << 20
)

// normalizeLogo turns an uploaded PNG, JPEG or GIF into a PNG that fits
// 400×240 (never enlarged) — small enough for every screen, and a "real
// PNG" the picon reader accepts.
func normalizeLogo(b []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxLogoSide || cfg.Height > maxLogoSide {
		return nil, errBadLogo
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, errBadLogo
	}
	w, h := cfg.Width, cfg.Height
	if w > storedWidth || h > storedHeight {
		scale := min(float64(storedWidth)/float64(w), float64(storedHeight)/float64(h))
		w, h = max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	}
	out := scaleBox(src, w, h)
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil || buf.Len() > maxStoredLogo {
		return nil, errBadLogo
	}
	return buf.Bytes(), nil
}

var errBadLogo = &logoError{"that isn't a usable picture (PNG, JPEG or GIF, not larger than 4000 px)"}

type logoError struct{ msg string }

func (e *logoError) Error() string { return e.msg }

// scaleBox averages the source pixels each target pixel covers (alpha
// weighted), which keeps a shrunken logo's edges clean.
func scaleBox(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if b.Dx() == w && b.Dy() == h {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}
	for y := 0; y < h; y++ {
		y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+(y+1)*b.Dy()/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+(x+1)*b.Dx()/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					pr, pg, pb, pa := src.At(xx, yy).RGBA()
					r, g, bl, a, n = r+uint64(pr), g+uint64(pg), bl+uint64(pb), a+uint64(pa), n+1
				}
			}
			if n == 0 || a == 0 {
				continue
			}
			// r,g,b are premultiplied sums; un-premultiply the average.
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r * 255 / a), G: uint8(g * 255 / a), B: uint8(bl * 255 / a), A: uint8(a / n >> 8),
			})
		}
	}
	return dst
}

// storeLogo writes ref's logo atomically.
func storeLogo(ref string, png []byte) error {
	dir, key := logoDirectory(), logoKey(ref)
	if dir == "" || key == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".logo-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(png); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, key+".png"))
}

// --- handlers -----------------------------------------------------------------

// channelPicon serves ?ref&name like the shared picon lookup, and with
// &set= only that set's picture — the chooser's thumbnails.
func (h *Channels) channelPicon(w http.ResponseWriter, r *http.Request) {
	set := r.URL.Query().Get("set")
	if set == "" {
		servePicon(w, r, h.PiconDirs)
		return
	}
	for _, s := range h.piconSets() {
		if s.Name != set {
			continue
		}
		if b, ok := readPiconIn([]string{s.Path}, r.URL.Query().Get("ref"), r.URL.Query().Get("name")); ok {
			writePNG(w, b)
			return
		}
	}
	httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no picon for this channel in that set")
}

func writePNG(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}

// logoSets answers GET /api/v1/channels/logo/sets?ref&name: the installed
// sets that have a picture for this channel.
func (h *Channels) logoSets(w http.ResponseWriter, r *http.Request) {
	ref, name := r.URL.Query().Get("ref"), r.URL.Query().Get("name")
	sets := []string{}
	for _, s := range h.piconSets() {
		if hasPiconIn([]string{s.Path}, ref, name) {
			sets = append(sets, s.Name)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sets": sets, "custom": logoKey(ref) != "" && hasLogoOverride(ref)})
}

func hasLogoOverride(ref string) bool { _, ok := readLogoOverride(ref); return ok }

// customLogos answers GET /api/v1/channels/logo/custom: which channels
// have a hand-chosen logo, as picon-style keys (1_0_19_2EE3_…).
func (h *Channels) customLogos(w http.ResponseWriter, r *http.Request) {
	keys := []string{}
	if dir := logoDirectory(); dir != "" {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if n := e.Name(); !e.IsDir() && strings.HasSuffix(n, ".png") && !strings.HasPrefix(n, ".") {
					keys = append(keys, strings.TrimSuffix(n, ".png"))
				}
			}
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (h *Channels) logoReady(w http.ResponseWriter, r *http.Request, ref string) bool {
	if logoDirectory() == "" || logoKey(ref) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "a channel's service reference is required")
		return false
	}
	return true
}

// chooseLogo answers PUT /api/v1/channels/logo {"ref","name","set"}: take
// the picture an installed set has for this channel.
func (h *Channels) chooseLogo(w http.ResponseWriter, r *http.Request) {
	var body struct{ Ref, Name, Set string }
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Set == "" || !h.logoReady(w, r, body.Ref) {
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"ref": "...", "name": "...", "set": "<set>"}`)
		}
		return
	}
	for _, s := range h.piconSets() {
		if s.Name != body.Set {
			continue
		}
		b, ok := readPiconIn([]string{s.Path}, body.Ref, body.Name)
		if !ok {
			break
		}
		if err := storeLogo(body.Ref, b); err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not save the logo")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"saved": true})
		return
	}
	httpx.WriteError(w, r, http.StatusNotFound, "not_found", "that set has no picture for this channel")
}

// uploadLogo answers POST /api/v1/channels/logo/upload?ref=: the body is
// a PNG, JPEG or GIF.
func (h *Channels) uploadLogo(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if !h.logoReady(w, r, ref) {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLogoUpload))
	if err != nil {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "logo_too_large", "the picture is larger than 4 MB")
		return
	}
	h.saveNormalized(w, r, ref, data)
}

func (h *Channels) saveNormalized(w http.ResponseWriter, r *http.Request, ref string, data []byte) {
	b, err := normalizeLogo(data)
	if err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "logo_invalid", err.Error())
		return
	}
	if err := storeLogo(ref, b); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not save the logo")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"saved": true})
}

// searchLogos answers GET /api/v1/channels/logo/search?name=: matches in
// the public TV-logo collection.
func (h *Channels) searchLogos(w http.ResponseWriter, r *http.Request) {
	if h.Logos == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "logos_unavailable", "searching for logos isn't available")
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" || len(name) > 100 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "name is required")
		return
	}
	found, err := h.Logos.Search(r.Context(), name, 8)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "logos_unreachable", "the logo collection didn't answer")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": found})
}

// previewLogo answers GET /api/v1/channels/logo/preview?path=: a result's
// picture, through DreamHub (the browser never talks to the collection).
func (h *Channels) previewLogo(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if h.Logos == nil || !logosearch.ValidPath(p) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such logo")
		return
	}
	b, err := h.Logos.Fetch(r.Context(), p)
	if err != nil || http.DetectContentType(b) != "image/png" {
		httpx.WriteError(w, r, http.StatusBadGateway, "logos_unreachable", "the logo collection didn't answer")
		return
	}
	writePNG(w, b)
}

// fetchLogo answers POST /api/v1/channels/logo/online {"ref","path"}.
func (h *Channels) fetchLogo(w http.ResponseWriter, r *http.Request) {
	var body struct{ Ref, Path string }
	if err := httpx.DecodeJSON(r, &body); err != nil || !logosearch.ValidPath(body.Path) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"ref": "...", "path": "countries/…png"}`)
		return
	}
	if h.Logos == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "logos_unavailable", "searching for logos isn't available")
		return
	}
	if !h.logoReady(w, r, body.Ref) {
		return
	}
	data, err := h.Logos.Fetch(r.Context(), body.Path)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "logos_unreachable", "the logo collection didn't answer")
		return
	}
	h.saveNormalized(w, r, body.Ref, data)
}

// resetLogo answers DELETE /api/v1/channels/logo?ref=: back to the
// automatic logo.
func (h *Channels) resetLogo(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if !h.logoReady(w, r, ref) {
		return
	}
	err := os.Remove(filepath.Join(logoDirectory(), logoKey(ref)+".png"))
	if err != nil && !os.IsNotExist(err) {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not remove the logo")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
