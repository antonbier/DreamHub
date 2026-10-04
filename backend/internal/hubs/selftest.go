package hubs

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/dmi"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/systemd"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// SelfTest is SystemHub's box self-test (System → Diagnostics): one press
// asks the box, read-only, everything DreamHub so far only knew from the
// mock or from convention — the radio root and radio EPG, picon names, the
// service unit names, SerienRecorder, AutoTimer, DreamMediaInfo — and
// rates each answer. The raw answers are kept next to the report (Dir) as
// test fixtures, and can be downloaded as one zip. Nothing on the box is
// changed: GETs to WebControl, `systemctl show`, directory listings.
// Admin only, one run at a time, the checks one after another so the
// single-threaded WebControl isn't flooded.
type SelfTest struct {
	WC        webcontrol.Client
	SR        serienrecorder.Client
	AT        autotimer.Client
	DMI       *dmi.Reader
	Systemd   systemd.Manager
	FS        recfs.FS
	PiconDirs []string // nil = DefaultPiconDirs
	// Dir receives report.json and the captured answers (DataDir/selftest).
	Dir string
	// Writes is DREAMHUB_ENABLE_DEVICE_WRITES, reported, not changed.
	Writes bool
	Now    func() time.Time

	running sync.Mutex
	mu      sync.Mutex
	last    *SelfTestReport
}

// Statuses of a check.
const (
	CheckOK   = "ok"
	CheckWarn = "warn" // works, but not as expected — worth a look
	CheckFail = "fail"
	CheckSkip = "skip" // not applicable here (a plugin not installed, no systemd)
)

// SelfTestCheck is one rated question to the box. Code and Params are the
// detail line (an i18n key under system.selftest.result).
type SelfTestCheck struct {
	ID     string         `json:"id"`
	Status string         `json:"status"`
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
	Millis int64          `json:"ms"`
}

// SelfTestReport is a whole run.
type SelfTestReport struct {
	StartedAt time.Time       `json:"startedAt"`
	Checks    []SelfTestCheck `json:"checks"`
	// Captured lists the files saved next to the report; empty in mock
	// mode, where there's no box to capture.
	Captured []string `json:"captured"`
}

type rawGetter interface {
	Raw(ctx context.Context, path string) ([]byte, error)
}

type rawShower interface {
	Show(ctx context.Context, names []string) ([]byte, error)
}

const checkTimeout = 10 * time.Second

// selfTestRun carries one run's state between the checks.
type selfTestRun struct {
	t        *SelfTest
	report   *SelfTestReport
	files    map[string][]byte
	tvFirst  string // first TV bouquet's reference
	radioRef string // first radio bouquet's reference
	tvRefs   []webcontrol.Service
	radio    []webcontrol.Service
}

func (r *selfTestRun) check(ctx context.Context, id string, fn func(ctx context.Context) (status, code string, params map[string]any)) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	start := time.Now()
	status, code, params := fn(ctx)
	r.report.Checks = append(r.report.Checks, SelfTestCheck{ID: id, Status: status, Code: code, Params: params, Millis: time.Since(start).Milliseconds()})
}

// capture keeps the box's raw answer to path under name, if the client
// can give it (the real one can, the mock can't).
func (r *selfTestRun) capture(ctx context.Context, name, path string) {
	raw, ok := r.t.WC.(rawGetter)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	if b, err := raw.Raw(ctx, path); err == nil {
		r.files[name] = b
	}
}

func getservices(ref string) string {
	return "/web/getservices?" + url.Values{"sRef": {ref}}.Encode()
}

// Run asks the box everything once and keeps the report.
func (t *SelfTest) Run(ctx context.Context) (*SelfTestReport, error) {
	if !t.running.TryLock() {
		return nil, errSelfTestRunning
	}
	defer t.running.Unlock()
	now := time.Now
	if t.Now != nil {
		now = t.Now
	}
	r := &selfTestRun{t: t, report: &SelfTestReport{StartedAt: now().UTC(), Checks: []SelfTestCheck{}, Captured: []string{}}, files: map[string][]byte{}}

	r.check(ctx, "box", func(ctx context.Context) (string, string, map[string]any) {
		info, err := t.WC.DeviceInfo(ctx)
		if err != nil {
			return CheckFail, "unreachable", map[string]any{"error": err.Error()}
		}
		return CheckOK, "box", map[string]any{"model": info.DeviceName, "image": info.ImageVersion, "enigma": info.EnigmaVersion, "tuners": info.Tuners()}
	})
	r.capture(ctx, "deviceinfo.xml", "/web/deviceinfo")
	boxDown := r.report.Checks[0].Status == CheckFail

	r.check(ctx, "writes", func(context.Context) (string, string, map[string]any) {
		if t.Writes {
			return CheckOK, "writesOn", nil
		}
		return CheckWarn, "writesOff", nil
	})

	if !boxDown {
		r.boxChecks(ctx)
	}
	r.localChecks(ctx, boxDown)

	if err := t.save(r); err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.last = r.report
	t.mu.Unlock()
	return r.report, nil
}

var errSelfTestRunning = errors.New("selftest: already running")

// boxChecks are the questions to WebControl and the plugins on the box.
func (r *selfTestRun) boxChecks(ctx context.Context) {
	t := r.t
	r.check(ctx, "tvBouquets", func(ctx context.Context) (string, string, map[string]any) {
		bouquets, err := t.WC.Bouquets(ctx)
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		if len(bouquets) == 0 {
			return CheckWarn, "noBouquets", nil
		}
		r.tvFirst = bouquets[0].ServiceReference
		services, err := t.WC.Services(ctx, r.tvFirst)
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		for _, s := range services {
			if !webcontrol.IsMarker(s.ServiceReference) {
				r.tvRefs = append(r.tvRefs, s)
			}
		}
		return CheckOK, "bouquets", map[string]any{"bouquets": len(bouquets), "first": bouquets[0].ServiceName, "channels": len(r.tvRefs)}
	})
	r.capture(ctx, "getservices.xml", "/web/getservices")

	if r.tvFirst != "" {
		r.check(ctx, "tvEpg", func(ctx context.Context) (string, string, map[string]any) {
			list, err := t.WC.EPGNowNext(ctx, r.tvFirst)
			if err != nil {
				return CheckFail, "error", map[string]any{"error": err.Error()}
			}
			with := 0
			for _, n := range list {
				if n.Now != nil {
					with++
				}
			}
			if with == 0 {
				return CheckWarn, "noEpg", map[string]any{"channels": len(list)}
			}
			return CheckOK, "epg", map[string]any{"withEpg": with, "channels": len(list)}
		})
	}

	r.check(ctx, "current", func(ctx context.Context) (string, string, map[string]any) {
		cur, err := t.WC.Current(ctx)
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		kind := "tv"
		switch {
		case webcontrol.IsRecordingReference(cur.ServiceReference):
			kind = "recording"
		case webcontrol.IsRadioService(cur.ServiceReference):
			kind = "radio"
		}
		return CheckOK, "current", map[string]any{"name": cur.ServiceName, "kind": kind, "epg": cur.Now != nil}
	})
	r.capture(ctx, "getcurrent.xml", "/web/getcurrent")
	r.capture(ctx, "powerstate.xml", "/web/powerstate")

	// The radio root is Enigma2's convention, never seen on this box yet.
	r.check(ctx, "radioBouquets", func(ctx context.Context) (string, string, map[string]any) {
		list, err := t.WC.Services(ctx, webcontrol.RadioRoot)
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		var bouquets []webcontrol.Service
		for _, b := range list {
			if !webcontrol.IsMarker(b.ServiceReference) {
				bouquets = append(bouquets, b)
			}
		}
		if len(bouquets) == 0 {
			return CheckWarn, "noRadioBouquets", nil
		}
		r.radioRef = bouquets[0].ServiceReference
		return CheckOK, "radioBouquets", map[string]any{"bouquets": len(bouquets), "first": bouquets[0].ServiceName}
	})
	r.capture(ctx, "getservices_radio_root.xml", getservices(webcontrol.RadioRoot))

	if r.radioRef != "" {
		r.check(ctx, "radioStations", func(ctx context.Context) (string, string, map[string]any) {
			services, err := t.WC.Services(ctx, r.radioRef)
			if err != nil {
				return CheckFail, "error", map[string]any{"error": err.Error()}
			}
			other := 0
			for _, s := range services {
				switch {
				case webcontrol.IsMarker(s.ServiceReference):
				case webcontrol.IsRadioService(s.ServiceReference):
					r.radio = append(r.radio, s)
				default:
					other++ // a stream or a type RadioHub doesn't recognize
				}
			}
			nowNext, err := t.WC.EPGNowNext(ctx, r.radioRef)
			with := 0
			if err == nil {
				for _, n := range nowNext {
					if n.Now != nil {
						with++
					}
				}
			}
			params := map[string]any{"stations": len(r.radio), "other": other, "withEpg": with}
			if len(r.radio) == 0 {
				return CheckWarn, "noRadioStations", params
			}
			if other > 0 {
				return CheckWarn, "radioOther", params
			}
			return CheckOK, "radioStations", params
		})
		r.capture(ctx, "getservices_radio_bouquet.xml", getservices(r.radioRef))
		r.capture(ctx, "epgnownext_radio.xml", "/web/epgnownext?"+url.Values{"bRef": {r.radioRef}}.Encode())
	}

	r.check(ctx, "timers", func(ctx context.Context) (string, string, map[string]any) {
		timers, err := t.WC.TimerList(ctx)
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		radio := 0
		for _, tm := range timers {
			if webcontrol.IsRadioService(tm.ServiceReference) {
				radio++
			}
		}
		return CheckOK, "timers", map[string]any{"timers": len(timers), "radio": radio}
	})
	r.capture(ctx, "timerlist.xml", "/web/timerlist")

	r.check(ctx, "recordings", func(ctx context.Context) (string, string, map[string]any) {
		folders, err := t.WC.Locations(ctx)
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		fileOps := t.FS != nil && recfs.Available(t.FS, folders)
		params := map[string]any{"folders": strings.Join(folders, ", "), "fileOps": fileOps}
		if !fileOps {
			return CheckWarn, "noFileOps", params
		}
		return CheckOK, "recordings", params
	})

	r.check(ctx, "serienrecorder", func(ctx context.Context) (string, string, map[string]any) {
		if t.SR == nil {
			return CheckSkip, "notConfigured", nil
		}
		info, err := t.SR.Info(ctx)
		if errors.Is(err, serienrecorder.ErrNotInstalled) {
			return CheckSkip, "notInstalled", nil
		}
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		markers, _ := t.SR.Markers(ctx)
		return CheckOK, "serienrecorder", map[string]any{"version": info.SRVersion, "api": info.APIVersion, "markers": len(markers)}
	})

	r.check(ctx, "autotimer", func(ctx context.Context) (string, string, map[string]any) {
		if t.AT == nil {
			return CheckSkip, "notConfigured", nil
		}
		list, err := t.AT.List(ctx)
		if errors.Is(err, autotimer.ErrNotInstalled) {
			return CheckSkip, "notInstalled", nil
		}
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		return CheckOK, "autotimer", map[string]any{"count": len(list)}
	})
}

// localChecks look at the box's own disk and systemd — DreamHub runs on
// the box, so these work even when WebControl doesn't answer.
func (r *selfTestRun) localChecks(ctx context.Context, boxDown bool) {
	t := r.t
	r.check(ctx, "picons", func(context.Context) (string, string, map[string]any) {
		dirs := t.PiconDirs
		if dirs == nil {
			dirs = DefaultPiconDirs
		}
		var listing strings.Builder
		var present []string
		for _, d := range dirs {
			entries, err := os.ReadDir(d)
			if err != nil {
				continue
			}
			present = append(present, d)
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			sort.Strings(names)
			listing.WriteString("# " + d + " (" + strconv.Itoa(len(names)) + " files)\n")
			for i, n := range names {
				if i == 40 {
					break
				}
				listing.WriteString(n + "\n")
			}
		}
		if len(present) > 0 {
			r.files["picons.txt"] = []byte(listing.String())
		}
		if len(present) == 0 {
			return CheckWarn, "noPiconDirs", map[string]any{"dirs": strings.Join(dirs, ", ")}
		}
		found, tried := matchPicons(dirs, r.tvRefs, 30)
		rfound, rtried := matchPicons(dirs, r.radio, 30)
		params := map[string]any{"dirs": strings.Join(present, ", "), "tv": found, "tvTried": tried, "radio": rfound, "radioTried": rtried}
		if tried > 0 && found == 0 {
			return CheckWarn, "piconsNoMatch", params
		}
		if boxDown {
			return CheckOK, "piconDirs", params
		}
		return CheckOK, "picons", params
	})

	r.check(ctx, "services", func(ctx context.Context) (string, string, map[string]any) {
		if t.Systemd == nil {
			return CheckSkip, "noSystemd", nil
		}
		units, err := t.Systemd.Status(ctx, DefaultServices)
		if errors.Is(err, systemd.ErrUnavailable) {
			return CheckSkip, "noSystemd", nil
		}
		if err != nil {
			return CheckFail, "error", map[string]any{"error": err.Error()}
		}
		var names, down []string
		memory, enigma := false, false
		for _, u := range units {
			names = append(names, strings.TrimSuffix(u.Name, ".service"))
			if u.Memory > 0 {
				memory = true
			}
			if u.Name == "enigma2.service" {
				enigma = true
			}
			if u.Active != "active" {
				down = append(down, strings.TrimSuffix(u.Name, ".service"))
			}
		}
		if sh, ok := t.Systemd.(rawShower); ok {
			if b, err := sh.Show(ctx, DefaultServices); err == nil {
				r.files["systemctl_show.txt"] = b
			}
		}
		params := map[string]any{"found": strings.Join(names, ", "), "memory": memory, "down": strings.Join(down, ", ")}
		if !enigma {
			return CheckWarn, "noEnigmaUnit", params
		}
		if len(down) > 0 {
			return CheckWarn, "servicesDown", params
		}
		return CheckOK, "services", params
	})

	r.check(ctx, "dmi", func(context.Context) (string, string, map[string]any) {
		if t.DMI == nil {
			return CheckSkip, "notConfigured", nil
		}
		if !t.DMI.Available() {
			return CheckSkip, "notInstalled", nil
		}
		return CheckOK, "dmi", nil
	})
}

// matchPicons counts how many of the first max services have a picon by
// the names GuideHub and RadioHub look for.
func matchPicons(dirs []string, services []webcontrol.Service, max int) (found, tried int) {
	for _, s := range services {
		if tried == max {
			break
		}
		tried++
		for _, name := range piconNames(s.ServiceReference, s.ServiceName) {
			hit := false
			for _, d := range dirs {
				if fi, err := os.Stat(filepath.Join(d, name)); err == nil && fi.Mode().IsRegular() {
					hit = true
					break
				}
			}
			if hit {
				found++
				break
			}
		}
	}
	return found, tried
}

// save replaces the previous run's files with this one's.
func (t *SelfTest) save(r *selfTestRun) error {
	if t.Dir == "" {
		return nil
	}
	if err := os.RemoveAll(t.Dir); err != nil {
		return err
	}
	if err := os.MkdirAll(t.Dir, 0o700); err != nil {
		return err
	}
	names := make([]string, 0, len(r.files))
	for name := range r.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(t.Dir, name), r.files[name], 0o600); err != nil {
			return err
		}
	}
	r.report.Captured = names
	b, err := json.MarshalIndent(r.report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(t.Dir, "report.json"), b, 0o600)
}

// Last is the most recent report: this process's, else the one on disk.
func (t *SelfTest) Last() *SelfTestReport {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last != nil {
		return t.last
	}
	b, err := os.ReadFile(filepath.Join(t.Dir, "report.json"))
	if err != nil {
		return nil
	}
	var rep SelfTestReport
	if json.Unmarshal(b, &rep) != nil {
		return nil
	}
	t.last = &rep
	return t.last
}

func (t *SelfTest) handleRun(w http.ResponseWriter, r *http.Request) {
	rep, err := t.Run(r.Context())
	if errors.Is(err, errSelfTestRunning) {
		httpx.WriteError(w, r, http.StatusConflict, "selftest_running", "a self-test is already running")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "selftest_failed", "could not save the self-test's report")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": rep})
}

func (t *SelfTest) handleLast(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": t.Last()})
}

// handleDownload zips the last run's directory: report.json plus the
// captured answers.
func (t *SelfTest) handleDownload(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(t.Dir)
	if err != nil || len(entries) == 0 {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no self-test has run yet")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="dreamhub-selftest.zip"`)
	zw := zip.NewWriter(w)
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(t.Dir, e.Name()))
		if err != nil {
			continue
		}
		f, err := zw.Create(e.Name())
		if err != nil {
			return
		}
		f.Write(b)
	}
	zw.Close()
}
