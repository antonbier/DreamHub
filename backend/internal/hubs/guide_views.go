package hubs

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// What GuideHub's other views need besides the grid (GuideHubView: grid,
// now & next, tonight, one channel): now/next for a whole bouquet in one
// call to the box, and channel logos.

func (h *Guide) nowNext(w http.ResponseWriter, r *http.Request) {
	bouquet := r.URL.Query().Get("bouquet")
	if bouquet == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "bouquet is required")
		return
	}
	list, err := h.WC.EPGNowNext(r.Context(), bouquet)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	if list == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"channels": []any{}})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"channels": list})
}

// DefaultPiconDirs are where boxes keep channel logos ("picons"): the
// reference box's pool on /data first (ARCHITECTURE.md), then the usual
// Enigma2 places.
var DefaultPiconDirs = []string{"/data/piconpool", "/usr/share/enigma2/picon", "/media/hdd/picon", "/media/usb/picon"}

var piconRef = regexp.MustCompile(`^[0-9A-Fa-f]{1,8}(:[0-9A-Fa-f]{1,8}){9}:?`)

// piconNames are the file names a channel's picon may have, by the common
// conventions: the service reference's first ten fields joined with "_"
// ("1_0_19_2B7A_3F3_1_C00000_0_0_0.png"), the same with service type 1
// (picon sets often only carry that), and the channel name in lower case
// without spaces or punctuation ("daserstehd.png").
func piconNames(ref, name string) []string {
	var out []string
	if m := piconRef.FindString(ref); m != "" {
		fields := strings.Split(strings.TrimSuffix(m, ":"), ":")
		out = append(out, strings.ToUpper(strings.Join(fields, "_"))+".png")
		if fields[2] != "1" {
			alt := append([]string{}, fields...)
			alt[2] = "1"
			out = append(out, strings.ToUpper(strings.Join(alt, "_"))+".png")
		}
	}
	var b strings.Builder
	for _, r := range strings.ToLower(strings.NewReplacer("&", "and", "+", "plus", "*", "star").Replace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String()+".png")
	}
	return out
}

// picon serves ?ref=<service reference>&name=<channel name>: the first
// picon found, read-only, a real PNG only. 404 off the box or without one
// — the screen then shows the channel's name alone.
func (h *Guide) picon(w http.ResponseWriter, r *http.Request) { servePicon(w, r, h.PiconDirs) }

// servePicon is the picon lookup shared by GuideHub and RadioHub (each
// serves it under its own prefix, so neither depends on the other being
// enabled). nil dirs means DefaultPiconDirs.
func servePicon(w http.ResponseWriter, r *http.Request, dirs []string) {
	if dirs == nil {
		dirs = DefaultPiconDirs
	}
	for _, name := range piconNames(r.URL.Query().Get("ref"), r.URL.Query().Get("name")) {
		for _, dir := range dirs {
			p := filepath.Join(dir, name)
			fi, err := os.Lstat(p)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
				continue
			}
			b, err := os.ReadFile(p)
			if err != nil || http.DetectContentType(b) != "image/png" {
				continue
			}
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "private, max-age=86400")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Write(b)
			return
		}
	}
	httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no picon for this channel")
}
