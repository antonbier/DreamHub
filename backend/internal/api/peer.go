package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/boxes"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/peer"
)

// The peer API: how the DreamHub of another box pairs with this one and
// asks what this one can do (docs/MULTIDEVICE_PLAN.md, M6). The asking end
// is the hub's box management (handlePairBox).

// peerInfo is what this DreamHub says about itself without a login.
func peerInfo(ctx context.Context, d Deps) peer.Info {
	info := peer.Info{Version: Version, API: peer.APILevel, MinHubAPI: peer.MinPeerAPI, Pairing: d.Peer != nil && d.Peer.Active()}
	if d.Boxes != nil {
		ctx, cancel := context.WithTimeout(box.With(ctx, d.Boxes.DefaultID()), 2*time.Second)
		defer cancel()
		if di, err := d.WebControl.DeviceInfo(ctx); err == nil {
			info.DeviceName = di.DeviceName
		}
	}
	return info
}

func handlePeerInfo(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, peerInfo(r.Context(), d))
	}
}

// handlePeerPair is public on purpose: the code is the credential. A wrong
// code counts against the code (peer.MaxAttempts).
func handlePeerPair(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Code    string `json:"code"`
			HubName string `json:"hubName"`
		}
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"code", "hubName"}`)
			return
		}
		token, err := d.Peer.Redeem(r.Context(), in.Code, in.HubName)
		switch {
		case errors.Is(err, peer.ErrNotPairing):
			httpx.WriteError(w, r, http.StatusConflict, "not_pairing", "no pairing code is shown on this box")
		case errors.Is(err, peer.ErrBadCode):
			httpx.WriteError(w, r, http.StatusForbidden, "bad_code", "wrong or expired code")
		case err != nil:
			d.Logger.Error("pairing failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not pair")
		default:
			httpx.WriteJSON(w, http.StatusOK, peer.PairResult{Token: token, Info: peerInfo(r.Context(), d)})
		}
	}
}

// handlePeerCode shows a new pairing code (status page and TV).
func handlePeerCode(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := d.Peer.Start(r.Context(), current(r))
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not create a code")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, c)
	}
}

// handlePeerCapabilities: what this machine can do on its own — the same
// answer for a full DreamHub and (one day) a slim agent.
func handlePeerCapabilities(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := box.With(r.Context(), d.Boxes.DefaultID())
		out := peer.Capabilities{Info: peerInfo(ctx, d), Capabilities: []string{}}
		if v, err := d.Settings.Values(ctx, current(r)); err == nil {
			out.Capabilities = d.Boxes.Capabilities(ctx, d.Connectors.Reports(ctx, v, d.Settings.Version()))
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// handlePairBox pairs a box with the DreamHub running on it.
func handlePairBox(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in boxes.PairInput
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"code", "url"}`)
			return
		}
		name, _ := os.Hostname()
		if name == "" {
			name = "DreamHub"
		}
		e, _, err := d.BoxManager.Pair(r.Context(), r.PathValue("id"), in, name)
		if writeBoxError(w, r, err) {
			return
		}
		logBoxChange(d, r, "box.pair", "Paired box "+e.Name)
		httpx.WriteJSON(w, http.StatusOK, viewBox(d, e, true, true))
	}
}

func handleUnpairBox(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := d.BoxManager.Unpair(r.Context(), r.PathValue("id"))
		if writeBoxError(w, r, err) {
			return
		}
		logBoxChange(d, r, "box.unpair", "Unpaired box "+e.Name)
		httpx.WriteJSON(w, http.StatusOK, viewBox(d, e, true, true))
	}
}
