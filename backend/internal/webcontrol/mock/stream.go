package mock

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// LiveStream plays StreamFile, a real MPEG-TS file on disk, for whatever
// service is asked for (DREAMHUB_MOCK_STREAM in mock mode; nothing ships
// with the repository, broadcasts aren't ours to redistribute). Without one
// the mock box has no tuner to stream from.
func (c *Client) LiveStream(ctx context.Context, serviceReference string) (io.ReadCloser, error) {
	c.mu.Lock()
	file := c.StreamFile
	c.LastStreamed = serviceReference
	c.mu.Unlock()
	if file == "" {
		return nil, webcontrol.ErrNotFound
	}
	return os.Open(file)
}

// RecordingFile serves a recording from FS (Range ignored: always whole).
func (c *Client) RecordingFile(ctx context.Context, path, rangeHeader string) (*webcontrol.FileResponse, error) {
	if !strings.HasSuffix(path, ".ts") {
		return nil, webcontrol.ErrNotFound
	}
	b, err := c.FS.ReadFile(path)
	if err != nil {
		return nil, webcontrol.ErrNotFound
	}
	return &webcontrol.FileResponse{Status: http.StatusOK, ContentLength: int64(len(b)), Body: io.NopCloser(bytes.NewReader(b))}, nil
}
