package dockerd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// This file is the helper's safety net for what compose-defined apps can
// ask for (docs/APPSHUB_PLAN.md, App12). It is enforced twice: AppsHub
// checks a plan before showing it (so the person sees the refusal), and the
// helper checks again when it creates the container (Server.handleCreate
// and Exec.Create) — the helper does not take the main backend's word for
// it. Nothing here can be switched off by a request field; the only knob is
// Elevated.Confirmed, which unlocks exactly the things the plan calls "needs
// an extra confirmation".

// Mount is one bind mount of a container.
type Mount struct {
	HostDir      string `json:"hostDir"`
	ContainerDir string `json:"containerDir"`
	ReadOnly     bool   `json:"readOnly,omitempty"`
}

// Elevated is everything beyond a plain container.
type Elevated struct {
	Privileged bool     `json:"privileged,omitempty"`
	CapAdd     []string `json:"capAdd,omitempty"`
	// Devices are "host[:container[:perms]]" below /dev.
	Devices []string `json:"devices,omitempty"`
	PidHost bool     `json:"pidHost,omitempty"`
	IpcHost bool     `json:"ipcHost,omitempty"`
	// Confirmed: the person explicitly confirmed the items above.
	Confirmed bool `json:"confirmed,omitempty"`
}

// Any reports whether anything beyond a plain container is asked for.
func (e Elevated) Any() bool {
	return e.Privileged || len(e.CapAdd) > 0 || len(e.Devices) > 0 || e.PidHost || e.IpcHost
}

// SpecError is a ContainerSpec the helper refuses. Code is stable
// (mount_system, mount_socket, mount_dreamhub, mount_storage,
// mount_outside, mount_unclean, mount_unresolvable, needs_confirmation,
// device_refused, invalid); Message is English.
type SpecError struct {
	Code    string
	Message string
	// Path is the offending path, when there is one.
	Path string
}

func (e *SpecError) Error() string { return "dockerd: refused: " + e.Message }

func refuse(code, path, format string, args ...any) *SpecError {
	return &SpecError{Code: code, Path: path, Message: fmt.Sprintf(format, args...)}
}

// IsRefusal reports whether err is (or wraps) a *SpecError.
func IsRefusal(err error) (*SpecError, bool) {
	var se *SpecError
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}

// StorageDirName is the name of an apps storage root's folder
// (/media/hdd/dreamhub-apps): it holds every app's data and passwords, so
// no mount may be, contain or lie inside it except an app's own folders.
const StorageDirName = "dreamhub-apps"

// MountPolicy decides which host paths a container may mount.
type MountPolicy struct {
	// Bases are the directories storage and free mounts live under; the
	// default is /media. A mount must be strictly below one.
	Bases []string
	// Protected are directories that must never be mounted, contained in a
	// mount, or lie inside one — DreamHub's own data directory.
	Protected []string
	// Resolve follows symlinks (tests); nil means the real file system.
	Resolve func(string) (string, error)
	// Exists reports whether a path exists (tests); nil means os.Lstat.
	Exists func(string) bool
}

// DefaultPolicy is the policy of the real helper: /media only.
func DefaultPolicy() MountPolicy { return MountPolicy{Bases: []string{"/media"}} }

func (p MountPolicy) bases() []string {
	if len(p.Bases) == 0 {
		return []string{"/media"}
	}
	return p.Bases
}

func (p MountPolicy) exists(s string) bool {
	if p.Exists != nil {
		return p.Exists(s)
	}
	_, err := os.Lstat(s)
	return err == nil
}

// systemDirs are never mountable, named so the refusal can say why. They
// would be refused anyway (everything outside the bases is); this list
// makes the message clear and keeps a misconfigured base from opening one.
var systemDirs = []string{"/etc", "/usr", "/var", "/boot", "/proc", "/sys", "/dev", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/run", "/root", "/home", "/opt", "/srv", "/data"}

func within(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

func strictlyWithin(p, dir string) bool { return p != dir && within(p, dir) }

// resolveExisting resolves symlinks in the longest existing prefix of p and
// appends the rest (which does not exist, so it holds no links).
func resolveExisting(p string) (string, error) {
	cur, rest := p, ""
	for {
		if _, err := os.Lstat(cur); err == nil {
			break
		}
		if cur == "/" || cur == "." {
			return p, nil
		}
		rest = path.Join(path.Base(cur), rest)
		cur = path.Dir(cur)
	}
	real, err := filepath.EvalSymlinks(cur)
	if err != nil {
		return "", err
	}
	return path.Join(real, rest), nil
}

func (p MountPolicy) resolve(s string) (string, error) {
	if p.Resolve != nil {
		return p.Resolve(s)
	}
	return resolveExisting(s)
}

var badPathChars = "\x00\n\r:,"

// CheckMount decides whether a container of app appID, whose storage root
// is root, may mount host. It checks the path as written and as the file
// system resolves it (symlinks), and both must pass.
func (p MountPolicy) CheckMount(root, appID, host string) error {
	if !path.IsAbs(host) || path.Clean(host) != host || strings.ContainsAny(host, badPathChars) || len(host) > 1024 {
		return refuse("mount_unclean", host, "%q must be a clean absolute path (no .., no trailing slash)", host)
	}
	if err := p.checkOne(root, appID, host, p.bases(), p.Protected); err != nil {
		return err
	}
	// The same decision on what the paths really are.
	rh, err := p.resolve(host)
	if err != nil {
		return refuse("mount_unresolvable", host, "%q could not be resolved", host)
	}
	rr := root
	if root != "" {
		if rr, err = p.resolve(root); err != nil {
			return refuse("mount_unresolvable", root, "the storage folder %q could not be resolved", root)
		}
	}
	var rb, rp []string
	for _, b := range p.bases() {
		r, err := p.resolve(b)
		if err != nil {
			continue // a base that doesn't exist allows nothing
		}
		rb = append(rb, r)
	}
	for _, d := range p.Protected {
		r, err := p.resolve(d)
		if err != nil {
			return refuse("mount_unresolvable", d, "%q could not be resolved", d)
		}
		rp = append(rp, r)
	}
	return p.checkOne(rr, appID, rh, rb, rp)
}

func (p MountPolicy) checkOne(root, appID, host string, bases, protected []string) error {
	if host == "/" {
		return refuse("mount_system", host, "the whole file system can't be mounted")
	}
	for _, d := range systemDirs {
		if within(host, d) {
			return refuse("mount_system", host, "%s belongs to the box's system and can't be mounted", d)
		}
	}
	base := path.Base(host)
	if strings.HasSuffix(base, "docker.sock") || strings.HasSuffix(base, "containerd.sock") || strings.HasSuffix(base, ".sock") && within(host, "/run") {
		return refuse("mount_socket", host, "the Docker socket can't be mounted into a container")
	}
	for _, d := range protected {
		if within(host, d) || within(d, host) {
			return refuse("mount_dreamhub", host, "%s holds DreamHub's own data and can't be mounted", d)
		}
	}
	rootOK := root != ""
	if rootOK {
		inBase := false
		for _, b := range bases {
			inBase = inBase || strictlyWithin(root, b)
		}
		rootOK = inBase
	}
	if rootOK {
		if within(host, root) {
			rel := strings.TrimPrefix(host, root)
			rel = strings.TrimPrefix(rel, "/")
			first, _, _ := strings.Cut(rel, "/")
			switch {
			case first == "":
				return refuse("mount_storage", host, "the apps' storage folder holds every app's data and passwords; mount only the app's own folders")
			case first == appID+".secrets.json":
				return refuse("mount_storage", host, "the app's generated passwords file can't be mounted")
			case first == appID || strings.HasPrefix(first, appID+"."):
				return nil
			}
			return refuse("mount_storage", host, "%s belongs to another app's storage", host)
		}
		if within(root, host) {
			return refuse("mount_storage", host, "%s contains the apps' storage folder (every app's data and passwords)", host)
		}
	}
	// Another storage root (another disk) or anything holding one.
	for _, part := range strings.Split(host, "/") {
		if part == StorageDirName {
			return refuse("mount_storage", host, "%s is inside an apps storage folder", host)
		}
	}
	if p.exists(path.Join(host, StorageDirName)) {
		return refuse("mount_storage", host, "%s contains an apps storage folder (every app's data and passwords)", host)
	}
	for _, b := range bases {
		if strictlyWithin(host, b) {
			return nil
		}
	}
	return refuse("mount_outside", host, "%s is outside what apps may mount (their own storage folders and paths below /media)", host)
}

// forbiddenDevices can't be passed to a container even when confirmed:
// raw memory, the kernel log and the box's own flash.
var forbiddenDevices = regexp.MustCompile(`^/dev/(mem|kmem|port|kmsg|root|mmcblk[0-9]+(p[0-9]+)?|mtd.*|ubi.*|nand.*)$`)

// CheckDevice checks one Elevated.Devices entry.
func CheckDevice(d string) error {
	parts := strings.Split(d, ":")
	if len(parts) > 3 || d == "" {
		return refuse("device_refused", d, "%q is not a device mapping", d)
	}
	for i, part := range parts {
		if i == 2 {
			if part == "" || strings.Trim(part, "rwm") != "" {
				return refuse("device_refused", d, "%q has invalid permissions", d)
			}
			continue
		}
		if !strings.HasPrefix(part, "/dev/") || path.Clean(part) != part || strings.ContainsAny(part, badPathChars) {
			return refuse("device_refused", d, "%q is not a clean path below /dev", part)
		}
	}
	if forbiddenDevices.MatchString(parts[0]) {
		return refuse("device_refused", d, "%s (raw memory, kernel log or the box's flash) can't be passed to a container", parts[0])
	}
	return nil
}

var (
	appIDRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	imageRe   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,254}$`)
	envKeyRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
	capNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,31}$`)
)

const (
	maxMounts  = 16
	maxEnv     = 256
	maxEnvSize = 64 << 10
	maxCmdArgs = 64
	maxArgLen  = 8192
)

// ValidateSpec is everything the helper checks about a create request
// before it touches Docker.
func ValidateSpec(spec ContainerSpec, policy MountPolicy) error {
	if !appIDRe.MatchString(spec.AppID) {
		return refuse("invalid", "", "the app id %q is not valid", spec.AppID)
	}
	if !ValidService(spec.Service) {
		return ErrInvalidService
	}
	if !ValidRoot(spec.Root) {
		return refuse("invalid", spec.Root, "the storage root %q is not valid", spec.Root)
	}
	if !imageRe.MatchString(spec.Image) || strings.Contains(spec.Image, "..") || strings.Contains(spec.Image, "//") {
		return refuse("invalid", "", "the image %q is not valid", spec.Image)
	}
	if len(spec.Env) > maxEnv {
		return refuse("invalid", "", "too many environment variables")
	}
	size := 0
	for k, v := range spec.Env {
		if !envKeyRe.MatchString(k) || strings.ContainsRune(v, 0) {
			return refuse("invalid", "", "the environment variable %q is not valid", k)
		}
		size += len(k) + len(v)
	}
	if size > maxEnvSize {
		return refuse("invalid", "", "the environment is too large")
	}
	if len(spec.Cmd) > maxCmdArgs {
		return refuse("invalid", "", "the command has too many arguments")
	}
	for _, a := range spec.Cmd {
		if len(a) > maxArgLen || strings.ContainsRune(a, 0) {
			return refuse("invalid", "", "the command has an invalid argument")
		}
	}
	if len(spec.Mounts) > maxMounts {
		return refuse("invalid", "", "too many mounts")
	}
	targets := map[string]bool{}
	for _, m := range spec.Mounts {
		if !path.IsAbs(m.ContainerDir) || path.Clean(m.ContainerDir) != m.ContainerDir || strings.ContainsAny(m.ContainerDir, badPathChars) || len(m.ContainerDir) > 512 {
			return refuse("invalid", m.ContainerDir, "the mount point %q in the container is not valid", m.ContainerDir)
		}
		if targets[m.ContainerDir] {
			return refuse("invalid", m.ContainerDir, "two mounts at %s", m.ContainerDir)
		}
		targets[m.ContainerDir] = true
		if err := policy.CheckMount(spec.Root, spec.AppID, m.HostDir); err != nil {
			return err
		}
	}
	e := spec.Elevated
	if e.Any() && !e.Confirmed {
		return refuse("needs_confirmation", "", "privileged mode, capabilities, devices or host PID/IPC need the person's explicit confirmation")
	}
	for _, c := range e.CapAdd {
		if !capNameRe.MatchString(c) {
			return refuse("invalid", "", "%q is not a capability name", c)
		}
	}
	for _, d := range e.Devices {
		if err := CheckDevice(d); err != nil {
			return err
		}
	}
	if len(e.CapAdd) > 32 || len(e.Devices) > 16 {
		return refuse("invalid", "", "too many capabilities or devices")
	}
	return nil
}
