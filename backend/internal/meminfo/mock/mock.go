// Package mock implements meminfo.Reader with a fixed value, so AppsHub's
// resource-budget check has something deterministic to test and develop
// against (PRINCIPLES.md, "Mock-first development").
package mock

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/meminfo"
)

// Reader always answers the same Info.
type Reader struct {
	Info meminfo.Info
}

var _ meminfo.Reader = Reader{}

// New returns a reader reporting the reference box's own 2 GB, with 1 GB
// available — generous enough that the curated catalog's small default
// apps fit, without pretending the box has desktop-class headroom.
func New() Reader {
	return Reader{Info: meminfo.Info{TotalBytes: 2 << 30, AvailableBytes: 1 << 30}}
}

func (r Reader) Read(ctx context.Context) (meminfo.Info, error) { return r.Info, nil }
