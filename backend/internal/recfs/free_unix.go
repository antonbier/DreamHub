//go:build !windows

package recfs

import "syscall"

// Free implements Freer: the bytes an unprivileged process may still write.
func (OS) Free(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
