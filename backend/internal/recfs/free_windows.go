//go:build windows

package recfs

import "errors"

// Free is not available on Windows (the demo build); callers treat the error as "unknown".
func (OS) Free(string) (uint64, error) { return 0, errors.New("free space unknown on this platform") }
