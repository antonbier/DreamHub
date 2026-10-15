// Package mock implements dockersetup.Setup in memory: a box with no
// Docker yet (like the reference box), where setup needs every step and
// then is ready.
package mock

import (
	"context"
	"strings"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/dockersetup"
)

// Setup is an in-memory dockersetup.Setup.
type Setup struct {
	mu    sync.Mutex
	state dockersetup.State
	// Fail makes Apply and Remove fail (tests).
	Fail bool
	// removedData: the last uninstall also deleted the app data (tests).
	removedData bool
}

var _ dockersetup.Setup = (*Setup)(nil)

// New is a box without Docker: overlay module missing, 4 GB free on /data.
func New() *Setup {
	return &Setup{state: dockersetup.State{Supported: true, FreeBytes: 4 << 30}}
}

// Ready is a box where setup already ran.
func Ready() *Setup {
	return &Setup{state: dockersetup.State{Supported: true, Version: dockersetup.Version, Overlay: true, Configured: true,
		DockerRunning: true, HelperRunning: true, Ready: true, FreeBytes: 3 << 30}}
}

func (s *Setup) Status(ctx context.Context) (dockersetup.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, nil
}

func (s *Setup) Plan(ctx context.Context) ([]dockersetup.Step, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state
	var steps []dockersetup.Step
	if !st.Overlay {
		steps = append(steps, dockersetup.Step{Kind: "overlay", Detail: map[string]string{"source": "official", "version": "1:4.9.269-dream-219-g6f9ca1e-r0.1"}})
	}
	if st.Version != dockersetup.Version {
		steps = append(steps, dockersetup.Step{Kind: "download", Detail: map[string]string{"version": dockersetup.Version, "to": dockersetup.BinDir}})
	}
	if !st.Configured {
		steps = append(steps, dockersetup.Step{Kind: "config", Detail: map[string]string{"file": dockersetup.ConfigFile, "dataRoot": dockersetup.DataRoot}})
	}
	if !st.DockerRunning || !st.HelperRunning {
		steps = append(steps, dockersetup.Step{Kind: "services"})
	}
	return steps, nil
}

func (s *Setup) Apply(ctx context.Context, steps []dockersetup.Step) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Fail {
		return "Downloaded Docker\nFAILED: checksum mismatch\n", errFailed
	}
	s.state = dockersetup.State{Supported: true, Version: dockersetup.Version, Overlay: true, Configured: true,
		DockerRunning: true, HelperRunning: true, Ready: true, FreeBytes: s.state.FreeBytes - 300<<20}
	return "overlay module loaded\nDownloaded Docker " + dockersetup.Version + ", checksum verified\nWrote " + dockersetup.ConfigFile + "\nDocker and DreamHub's Docker helper are running\n", nil
}

// PlanRemove: everything is there while Docker is set up; afterwards
// nothing is left (a second uninstall has nothing to do).
func (s *Setup) PlanRemove(ctx context.Context, opts dockersetup.RemoveOptions) ([]dockersetup.Step, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state
	var steps []dockersetup.Step
	if st.DockerRunning || st.HelperRunning {
		steps = append(steps, dockersetup.Step{Kind: "services", Detail: map[string]string{"docker": dockersetup.DockerService, "helper": dockersetup.HelperService}})
	}
	if st.Overlay {
		steps = append(steps, dockersetup.Step{Kind: "modules", Detail: map[string]string{"file": dockersetup.ModulesFile}})
	}
	if st.Version != "" || st.Configured {
		steps = append(steps, dockersetup.Step{Kind: "files", Detail: map[string]string{"paths": dockersetup.BinDir + "," + dockersetup.ConfigFile}})
	}
	if opts.DeleteData && st.Version != "" {
		paths := append([]string{dockersetup.DataRoot}, opts.AppRoots...)
		steps = append(steps, dockersetup.Step{Kind: "data", Detail: map[string]string{"paths": strings.Join(paths, ",")}})
	}
	return steps, nil
}

func (s *Setup) Remove(ctx context.Context, steps []dockersetup.Step, opts dockersetup.RemoveOptions) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Fail {
		return "Docker stopped\nFAILED: could not remove the Docker files\n", errFailed
	}
	free := s.state.FreeBytes
	if opts.DeleteData {
		free += 300 << 20
	}
	s.state = dockersetup.State{Supported: true, FreeBytes: free}
	s.removedData = opts.DeleteData
	log := "Docker and DreamHub's Docker helper stopped and switched off\nRemoved " + dockersetup.ModulesFile + "\nRemoved the Docker files\n"
	if opts.DeleteData {
		log += "Removed the app data\n"
	}
	return log, nil
}

type failed struct{}

func (failed) Error() string { return "mock: setup failed" }

var errFailed = failed{}

// Gate wraps a Docker client so it answers as unreachable until s is
// ready — the mock of a box where Docker isn't set up yet.
func Gate(c dockerd.Client, s *Setup) dockerd.Client { return gated{Client: c, s: s} }

type gated struct {
	dockerd.Client
	s *Setup
}

func (g gated) ready() bool {
	st, _ := g.s.Status(context.Background())
	return st.Ready
}

func (g gated) List(ctx context.Context) ([]dockerd.Container, error) {
	if !g.ready() {
		return nil, dockerd.ErrUnavailable
	}
	return g.Client.List(ctx)
}

func (g gated) Create(ctx context.Context, spec dockerd.ContainerSpec) (dockerd.Container, error) {
	if !g.ready() {
		return dockerd.Container{}, dockerd.ErrUnavailable
	}
	return g.Client.Create(ctx, spec)
}

// RemovedData reports whether the last Remove deleted the app data.
func (s *Setup) RemovedData() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removedData
}
