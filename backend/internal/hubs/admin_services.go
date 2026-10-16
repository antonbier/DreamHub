package hubs

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/systemd"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// AdminHub's service status (ROADMAP.md, v1.1): the box's services —
// enigma2, DreamHub itself and the others found running on the reference
// box — with their state, and restarting one (or starting one that's
// down) through preview and confirm like every other change to the
// system. Only the units listed here can be restarted, nothing passed in
// freely.

// DefaultServices are the units shown, in this order; ones a box doesn't
// have are left out. Names differ between images, so alternatives are
// listed: on the reference box (DreamOS 2.6) Samba is smb/nmb, ZeroTier
// zerotierone and the softcam oscam-cycecplus (confirmed live 2026-10-04,
// ARCHITECTURE.md); smbd/nmbd, zerotier-one and oscam are the names
// elsewhere. Socket-activated units (dropbear, vsftpd) aren't listed: they
// read "inactive" while working fine.
var DefaultServices = []string{
	"enigma2.service",
	"dreamhub.service",
	// DreamHub's optional parts: their logs are where an extension's
	// "something went wrong" is explained.
	"dreamhub-docker.service",
	"dreamhub-dockerd-helper.service",
	"dreamhub-ext-ewelink.service",
	"smb.service",
	"nmb.service",
	"smbd.service",
	"nmbd.service",
	"nfs-server.service",
	"avahi-daemon.service",
	"zerotierone.service",
	"zerotier-one.service",
	"oscam-cycecplus.service",
	"oscam.service",
}

// Warning codes on a restart preview.
const (
	// Restarting enigma2 takes the TV picture, WebControl and the remote
	// away for a while.
	CodeServiceRestartEnigma2 = "service.restartEnigma2"
	// Recordings running right now would be cut (params: names).
	CodeServiceRecordingsCut = "service.recordingsInterrupted"
	// Restarting DreamHub: this page reconnects by itself.
	CodeServiceRestartSelf = "service.restartSelf"
)

func (h *Admin) services() []string {
	if h.Services != nil {
		return h.Services
	}
	return DefaultServices
}

func (h *Admin) listServices(w http.ResponseWriter, r *http.Request) {
	if h.Systemd == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "systemd_unavailable", "systemctl isn't available on this system")
		return
	}
	units, err := h.Systemd.Status(r.Context(), h.services())
	if errors.Is(err, systemd.ErrUnavailable) {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "systemd_unavailable", "systemctl isn't available on this system")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "systemd_failed", "could not read the services' state")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"services": units})
}

// unitFingerprint is what a restart plan was previewed against: the unit's
// state and since when — another restart in between makes it stale.
func unitFingerprint(u systemd.Unit) string {
	return u.Active + "|" + u.Sub + "|" + strconv.FormatInt(u.Since.Unix(), 10) + "|" + strconv.Itoa(u.MainPID)
}

func (h *Admin) unit(ctx context.Context, name string) (systemd.Unit, bool, error) {
	units, err := h.Systemd.Status(ctx, []string{name})
	if err != nil || len(units) == 0 {
		return systemd.Unit{}, false, err
	}
	return units[0], true, nil
}

func (h *Admin) planRestart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Name == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name": "<unit>.service"} as listed by GET /api/v1/admin/services`)
		return
	}
	if h.Systemd == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "systemd_unavailable", "systemctl isn't available on this system")
		return
	}
	if !slices.Contains(h.services(), body.Name) {
		httpx.WriteError(w, r, http.StatusNotFound, "service_not_found", "not one of the services DreamHub manages")
		return
	}
	ctx := r.Context()
	u, ok, err := h.unit(ctx, body.Name)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "systemd_failed", "could not read the service's state")
		return
	}
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "service_not_found", "this box has no such service")
		return
	}

	var warnings []status.Problem
	switch body.Name {
	case "enigma2.service":
		warnings = append(warnings, status.Problem{Code: CodeServiceRestartEnigma2})
		if names := h.runningRecordings(ctx); len(names) > 0 {
			warnings = append(warnings, status.Problem{Code: CodeServiceRecordingsCut, Params: map[string]any{"names": strings.Join(names, ", "), "count": len(names)}})
		}
	case "dreamhub.service":
		warnings = append(warnings, status.Problem{Code: CodeServiceRestartSelf})
	}

	action, summary := "service.restart", "Restarted service: "
	if u.Active != "active" {
		action, summary = "service.start", "Started service: "
	}
	user, _ := httpx.User(ctx)
	name := body.Name
	pl := h.Plans.Create(user, plan.Draft{
		Action:      action,
		Changes:     []plan.Change{{Type: action, Before: u}},
		Warnings:    warnings,
		Summary:     summary + name,
		Detail:      u,
		Fingerprint: unitFingerprint(u),
		Current: func(ctx context.Context) (string, error) {
			now, ok, err := h.unit(ctx, name)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", nil
			}
			return unitFingerprint(now), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return webcontrol.ErrWritesDisabled
			}
			if err := h.Systemd.Restart(ctx, name); err != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "service_restart_failed", Message: "systemctl couldn't restart the service", Err: err}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// runningRecordings are the timers recording right now — a restart of
// enigma2 cuts them. Not knowing (the box doesn't answer) means no warning
// rather than a failed preview: enigma2 being down is a common reason to
// restart it.
func (h *Admin) runningRecordings(ctx context.Context) []string {
	if h.WC == nil {
		return nil
	}
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		return nil
	}
	now := h.now().Unix()
	var names []string
	for _, t := range timers {
		if t.Disabled || t.JustPlay {
			continue
		}
		if t.Active() || (t.Begin <= now && now < t.End) {
			names = append(names, t.Name)
		}
	}
	return names
}
