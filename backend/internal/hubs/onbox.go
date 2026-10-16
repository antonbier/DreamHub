package hubs

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
)

// onBox is recfs.Available for the box the request is about: DreamHub's own
// file system is that box's disk only when the box is the machine DreamHub
// runs on (docs/MULTIDEVICE_PLAN.md, M1).
func onBox(ctx context.Context, fsys recfs.FS, roots []string) bool {
	return !box.Remote(ctx) && recfs.Available(fsys, roots)
}
