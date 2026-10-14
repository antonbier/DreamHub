package api_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const logoRef = "1:0:19:2EE3:441:1:C00000:0:0:0:"

func (e *env) logoGET(t *testing.T, path string) (*http.Response, []byte) {
	t.Helper()
	return e.do(t, "GET", path, "", nil)
}

func TestChannelLogos_ChooseFromTheInternetAndReset(t *testing.T) {
	e := newEnv(t)
	pic := "/api/v1/channels/picon?ref=" + url.QueryEscape(logoRef) + "&name=RTL"

	// Nothing yet: the mock box has no picons.
	if resp, _ := e.logoGET(t, pic); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("before: %d", resp.StatusCode)
	}

	resp, b := e.logoGET(t, "/api/v1/channels/logo/search?name=RTL+Television")
	var found struct {
		Results []struct{ Path string } `json:"results"`
	}
	json.Unmarshal(b, &found)
	if resp.StatusCode != http.StatusOK || len(found.Results) == 0 {
		t.Fatalf("search: %d %s", resp.StatusCode, b)
	}
	if resp, b := e.logoGET(t, "/api/v1/channels/logo/preview?path="+url.QueryEscape(found.Results[0].Path)); resp.StatusCode != http.StatusOK || !bytes.HasPrefix(b, []byte("\x89PNG")) {
		t.Fatalf("preview: %d", resp.StatusCode)
	}

	body, _ := json.Marshal(map[string]string{"ref": logoRef, "path": found.Results[0].Path})
	if resp, b := e.do(t, "POST", "/api/v1/channels/logo/online", string(body), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("choose: %d %s", resp.StatusCode, b)
	}
	// Now every hub's logo lookup finds it, and the check no longer lists it.
	if resp, b := e.logoGET(t, pic); resp.StatusCode != http.StatusOK || !bytes.HasPrefix(b, []byte("\x89PNG")) {
		t.Fatalf("after: %d", resp.StatusCode)
	}
	if _, b := e.logoGET(t, "/api/v1/channels/logo/custom"); !strings.Contains(string(b), "1_0_19_2EE3_441_1_C00000_0_0_0") {
		t.Fatalf("custom: %s", b)
	}

	if resp, _ := e.do(t, "DELETE", "/api/v1/channels/logo?ref="+url.QueryEscape(logoRef), "", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset: %d", resp.StatusCode)
	}
	if resp, _ := e.logoGET(t, pic); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after reset: %d", resp.StatusCode)
	}
}

func TestChannelLogos_UploadIsShrunkToAPNG(t *testing.T) {
	e := newEnv(t)
	img := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	resp, b := e.doRaw(t, "POST", "/api/v1/channels/logo/upload?ref="+url.QueryEscape(logoRef), buf.Bytes(), "image/jpeg")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload: %d %s", resp.StatusCode, b)
	}
	_, got := e.logoGET(t, "/api/v1/channels/picon?ref="+url.QueryEscape(logoRef)+"&name=x")
	cfg, format, err := image.DecodeConfig(bytes.NewReader(got))
	if err != nil || format != "png" || cfg.Width != 360 && cfg.Width != 400 || cfg.Height > 240 || cfg.Width > 400 {
		t.Fatalf("stored: %v %s %+v", err, format, cfg)
	}
	// Not a picture, or not a channel: refused.
	if resp, _ := e.doRaw(t, "POST", "/api/v1/channels/logo/upload?ref="+url.QueryEscape(logoRef), []byte("hello"), "text/plain"); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("garbage: %d", resp.StatusCode)
	}
	if resp, _ := e.doRaw(t, "POST", "/api/v1/channels/logo/upload?ref=../../etc", buf.Bytes(), "image/jpeg"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad ref: %d", resp.StatusCode)
	}
	// Only the collection's own files can be fetched.
	body, _ := json.Marshal(map[string]string{"ref": logoRef, "path": "../../etc/passwd"})
	if resp, _ := e.do(t, "POST", "/api/v1/channels/logo/online", string(body), nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad path: %d", resp.StatusCode)
	}
}

// doRaw posts a body that isn't JSON (an uploaded picture).
func (e *env) doRaw(t *testing.T, method, path string, body []byte, contentType string) (*http.Response, []byte) {
	t.Helper()
	return e.do(t, method, path, string(body), func(r *http.Request) { r.Header.Set("Content-Type", contentType) })
}

// A choose request without a set used to answer an empty 200.
func TestChannelLogos_ChooseNeedsASet(t *testing.T) {
	e := newEnv(t)
	body, _ := json.Marshal(map[string]string{"ref": logoRef, "name": "RTL"})
	resp, b := e.do(t, "PUT", "/api/v1/channels/logo", string(body), nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no set: %d %s", resp.StatusCode, b)
	}
}
