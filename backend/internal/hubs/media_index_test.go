package hubs

import (
	"context"
	"io/fs"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// slowFS holds ReadDir until released, so a first index build stays running.
type slowFS struct {
	recfs.FS
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *slowFS) ReadDir(name string) ([]fs.DirEntry, error) {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return s.FS.ReadDir(name)
}

func TestRecordingIndexConcurrentFirstCallersWait(t *testing.T) {
	wc := mock.New()
	roots, _ := wc.Locations(context.Background())
	mem := recfs.NewMem(map[string]string{path.Join(roots[0], "a.ts"): "x"})
	slow := &slowFS{FS: mem, started: make(chan struct{}), release: make(chan struct{})}
	h := &Media{WC: wc, FS: slow}

	type res struct {
		e  []indexEntry
		ok bool
	}
	first := make(chan res, 1)
	go func() { e, ok := h.recordingIndex(context.Background()); first <- res{e, ok} }()
	<-slow.started
	second := make(chan res, 1)
	go func() { e, ok := h.recordingIndex(context.Background()); second <- res{e, ok} }()
	select {
	case <-second:
		t.Fatal("second caller returned before the first build finished")
	case <-time.After(100 * time.Millisecond):
	}
	close(slow.release)
	for i, ch := range []chan res{first, second} {
		r := <-ch
		if !r.ok || len(r.e) != 1 {
			t.Fatalf("caller %d: ok=%v entries=%d", i, r.ok, len(r.e))
		}
	}
}
