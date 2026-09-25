package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/journald"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func TestAdminLogs_FiltersByLevelAndText(t *testing.T) {
	e := newEnv(t)

	resp, b := e.do(t, "GET", "/api/v1/admin/logs", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var all struct {
		Entries []struct {
			Unit, Level, Message string
		} `json:"entries"`
	}
	if err := json.Unmarshal(b, &all); err != nil {
		t.Fatal(err)
	}
	if len(all.Entries) == 0 {
		t.Fatalf("expected mock journal entries, got none: %s", b)
	}

	_, b2 := e.do(t, "GET", "/api/v1/admin/logs?level=err", "", nil)
	var errOnly struct {
		Entries []struct{ Level string } `json:"entries"`
	}
	json.Unmarshal(b2, &errOnly)
	for _, entry := range errOnly.Entries {
		if entry.Level != "err" && entry.Level != "crit" && entry.Level != "alert" && entry.Level != "emerg" {
			t.Errorf("level=err must exclude %q", entry.Level)
		}
	}
	if len(errOnly.Entries) == 0 {
		t.Fatalf("expected at least the mock's err entry, got none")
	}

	_, b3 := e.do(t, "GET", "/api/v1/admin/logs?unit=dreamhub.service", "", nil)
	var byUnit struct {
		Entries []struct{ Unit string } `json:"entries"`
	}
	json.Unmarshal(b3, &byUnit)
	for _, entry := range byUnit.Entries {
		if entry.Unit != "dreamhub.service" {
			t.Errorf("unit filter must exclude %q", entry.Unit)
		}
	}
}

func TestAdminCrashLogs_ListsAndRedactsSecrets(t *testing.T) {
	m := mock.New()
	m.FS.Put("/media/hdd/enigma2_crash_1774122941.log", "Traceback (most recent call last):\npassword: hunter2\nRuntimeError: boom\n")
	e := newEnvWith(t, m)

	resp, b := e.do(t, "GET", "/api/v1/admin/crashlogs", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	var list struct {
		Logs []struct {
			Name string `json:"name"`
			At   string `json:"at"`
		} `json:"logs"`
	}
	json.Unmarshal(b, &list)
	if len(list.Logs) != 1 || list.Logs[0].Name != "enigma2_crash_1774122941.log" {
		t.Fatalf("expected exactly the one crash log fixture, got %+v", list.Logs)
	}
	if list.Logs[0].At == "" {
		t.Errorf("expected the timestamp parsed from the file name")
	}

	_, cb := e.do(t, "GET", "/api/v1/admin/crashlogs/enigma2_crash_1774122941.log", "", nil)
	var content struct{ Content string }
	json.Unmarshal(cb, &content)
	if strings.Contains(content.Content, "hunter2") {
		t.Errorf("crash log download must redact secrets, got %q", content.Content)
	}
	if !strings.Contains(content.Content, "RuntimeError: boom") {
		t.Errorf("crash log download must keep the rest of the content, got %q", content.Content)
	}
}

func TestAdminCrashLogs_EmptyWithoutRecfs(t *testing.T) {
	e := newEnvWithOptions(t, mock.New(), nil, nil) // no RecordingsFS at all
	resp, b := e.do(t, "GET", "/api/v1/admin/crashlogs", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 even without a file system, got %d: %s", resp.StatusCode, b)
	}
	var list struct {
		Logs []any `json:"logs"`
	}
	json.Unmarshal(b, &list)
	if len(list.Logs) != 0 {
		t.Errorf("expected an empty list, got %+v", list.Logs)
	}
}

// mutableLogReader lets a test control what journalctl "returns" between
// polls, for exercising logsStream's poll-and-dedupe live tail.
type mutableLogReader struct {
	mu      sync.Mutex
	entries []journald.Entry
}

func (r *mutableLogReader) set(entries []journald.Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = entries
}

func (r *mutableLogReader) Read(context.Context, journald.Query) ([]journald.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]journald.Entry, len(r.entries))
	copy(out, r.entries)
	return out, nil
}

func TestAdminLogsStream_PushesNewLinesWithoutRepeatingOldOnes(t *testing.T) {
	reader := &mutableLogReader{}
	e := newEnvWithLogs(t, reader)

	reader.set([]journald.Entry{{Time: time.Unix(1, 0), Unit: "dreamhub.service", Level: journald.LevelInfo, Message: "first"}})

	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/admin/logs/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	lines := make(chan string, 10)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data:") {
				lines <- sc.Text()
			}
		}
	}()

	waitFor := func(sub string) {
		t.Helper()
		deadline := time.After(5 * time.Second) // logStreamInterval is 3s between polls
		for {
			select {
			case l := <-lines:
				if strings.Contains(l, sub) {
					return
				}
			case <-deadline:
				t.Fatalf("timed out waiting for a line containing %q", sub)
			}
		}
	}
	waitFor("first")

	reader.set([]journald.Entry{
		{Time: time.Unix(1, 0), Unit: "dreamhub.service", Level: journald.LevelInfo, Message: "first"},
		{Time: time.Unix(2, 0), Unit: "dreamhub.service", Level: journald.LevelInfo, Message: "second"},
	})
	waitFor("second")

	// "first" must not have been pushed a second time once "second" showed
	// up — drain what's buffered and make sure there's no repeat.
	time.Sleep(50 * time.Millisecond)
	for {
		select {
		case l := <-lines:
			if strings.Contains(l, `"first"`) {
				t.Fatalf("must not re-push an already-sent line: %s", l)
			}
		default:
			return
		}
	}
}
