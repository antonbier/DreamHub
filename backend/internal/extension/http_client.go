package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// callTimeout bounds one call to an extension's process — long enough for
// a real action (an API call to a smart-plug cloud service, say), short
// enough that a hung extension doesn't tie up a GET /api/v1/extensions
// request forever.
const callTimeout = 5 * time.Second

// HTTPClient reaches a registered extension's process on its own
// localhost address — the "safer, more consistent with how PlexHub
// already proxies Plex" choice from EXTENSIONHUB_PLAN.md's open
// questions: the extension's process binds only to localhost
// (docs/EXTENSIONHUB_PLAN.md, E3), and DreamHub's own backend is the only
// thing that ever calls it directly. Nothing extension-related is
// reachable except through DreamHub's own auth and permission checks.
//
// The three calls an extension's process must answer, all under BaseURL:
//   - GET  /widgets                      -> 200 []Widget
//   - POST /actions/{id}/plan            -> 200 ActionPreview
//   - POST /actions/{id}/apply {fingerprint} -> 200, or 409 (stale) / 4xx-5xx
type HTTPClient struct {
	BaseURL string
	// Transport overrides http.DefaultTransport (tests).
	Transport http.RoundTripper
}

var _ Client = HTTPClient{}

func (c HTTPClient) client() *http.Client {
	return &http.Client{Timeout: callTimeout, Transport: c.Transport}
}

func (c HTTPClient) Widgets(ctx context.Context) ([]Widget, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/widgets", nil)
	if err != nil {
		return nil, err
	}
	var widgets []Widget
	if err := c.doJSON(req, &widgets); err != nil {
		return nil, err
	}
	return widgets, nil
}

func (c HTTPClient) PlanAction(ctx context.Context, actionID string) (ActionPreview, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/actions/"+actionID+"/plan", nil)
	if err != nil {
		return ActionPreview{}, err
	}
	var preview ActionPreview
	if err := c.doJSON(req, &preview); err != nil {
		return ActionPreview{}, err
	}
	return preview, nil
}

// ErrStale means the extension itself rejected the fingerprint as no
// longer current.
var ErrStale = errors.New("extension: stale fingerprint")

func (c HTTPClient) Apply(ctx context.Context, actionID, fingerprint string) error {
	body, err := json.Marshal(map[string]string{"fingerprint": fingerprint})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/actions/"+actionID+"/apply", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("extension: apply: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return ErrStale
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("extension: apply: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (c HTTPClient) doJSON(req *http.Request, dst any) error {
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("extension: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("extension: unexpected status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
