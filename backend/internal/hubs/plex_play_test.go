package hubs

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

// blockingReader never returns from Read until closed, like a Plex
// connection whose data has stopped arriving mid-stream.
type blockingReader struct {
	closed chan struct{}
}

func newBlockingReader() *blockingReader { return &blockingReader{closed: make(chan struct{})} }

func (r *blockingReader) Read([]byte) (int, error) {
	<-r.closed
	return 0, errors.New("closed")
}

func (r *blockingReader) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

func TestCopyUntilStall_GivesUpAndClosesAStalledSource(t *testing.T) {
	src := newBlockingReader()
	var dst bytes.Buffer
	done := make(chan struct{})
	go func() {
		copyUntilStall(&dst, src, 20*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("copyUntilStall didn't give up on a stalled source")
	}
	select {
	case <-src.closed:
	default:
		t.Error("the stalled source wasn't closed")
	}
}

func TestCopyUntilStall_CopiesEverythingWhenNotStalled(t *testing.T) {
	src := io.NopCloser(bytes.NewReader([]byte("hello, world")))
	var dst bytes.Buffer
	copyUntilStall(&dst, src, time.Second)
	if dst.String() != "hello, world" {
		t.Errorf("copied %q", dst.String())
	}
}
