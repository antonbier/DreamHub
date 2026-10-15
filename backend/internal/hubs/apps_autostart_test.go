package hubs

import (
	"context"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	dockermock "github.com/antonbier/DreamHub/backend/internal/dockerd/mock"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

func newSettingsApps(t *testing.T) *Apps {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	user, err := st.EnsureDefaultUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reg := settings.NewRegistry()
	if err := reg.Register(AppsSettingsSection()); err != nil {
		t.Fatal(err)
	}
	svc, err := settings.NewService(ctx, st, reg)
	if err != nil {
		t.Fatal(err)
	}
	return &Apps{Docker: dockermock.New(), Settings: svc, User: user}
}

func TestAppsConfig_Defaults(t *testing.T) {
	h := newSettingsApps(t)
	c := h.config(context.Background())
	if !c.AutostartDefault || !c.UpdateNotify || c.UpdateHours != 24 || c.MemoryMB != 0 || c.StorageRoot != "" || len(c.Autostart) != 0 {
		t.Fatalf("defaults: %+v", c)
	}
	if c.defaultMemoryMB() != defaultMemoryLimitMB {
		t.Errorf("automatic memory is %d", c.defaultMemoryMB())
	}
}

func TestApps_AutostartPassOncePerBoot(t *testing.T) {
	ctx := context.Background()
	h := newSettingsApps(t)
	boot := "boot-1"
	h.BootID = func() string { return boot }
	// adguardhome is stopped in the mock; pihole runs.
	if err := h.setAutostart(ctx, "adguardhome", true); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		list, _ := h.Docker.List(ctx)
		for _, c := range list {
			if c.AppID() == "adguardhome" {
				return c.State
			}
		}
		return ""
	}
	if !h.autostartPass(ctx, nil) || state() != "running" {
		t.Fatalf("the pass should start the flagged, stopped app (state %q)", state())
	}
	// Somebody stops it; DreamHub restarts on the same boot: left alone.
	_ = h.Docker.Stop(ctx, "adguardhome")
	h.autostartPass(ctx, nil)
	if state() != "exited" {
		t.Fatal("an app stopped by hand was started again on the same boot")
	}
	// The next boot starts it again.
	boot = "boot-2"
	h.autostartPass(ctx, nil)
	if state() != "running" {
		t.Fatal("the next boot did not start it")
	}
	// Switched off: not started.
	_ = h.Docker.Stop(ctx, "adguardhome")
	_ = h.setAutostart(ctx, "adguardhome", false)
	boot = "boot-3"
	h.autostartPass(ctx, nil)
	if state() != "exited" || len(h.config(ctx).Autostart) != 0 {
		t.Fatal("an app without auto-start was started")
	}
}

func TestApps_AutostartPassWaitsForDocker(t *testing.T) {
	h := newSettingsApps(t)
	h.BootID = func() string { return "b" }
	h.Docker = failingList{dockermock.New()}
	if h.autostartPass(context.Background(), nil) {
		t.Fatal("must report that Docker did not answer yet, and not record the boot")
	}
	if h.config(context.Background()).BootID != "" {
		t.Fatal("the boot was recorded before Docker answered")
	}
}

func TestUpdateDue(t *testing.T) {
	now := time.Now()
	if updateDue(now.Add(-23*time.Hour), now, 24) || !updateDue(now.Add(-25*time.Hour), now, 24) {
		t.Fatal("interval not respected")
	}
}

type failingList struct{ *dockermock.Client }

func (failingList) List(context.Context) ([]dockerd.Container, error) {
	return nil, dockerd.ErrUnavailable
}
