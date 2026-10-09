package recfs

import "syscall"

// Freer is an FS that knows the free space where a path lives — for work
// that writes a large new file next to a recording (trimming).
type Freer interface {
	Free(path string) (uint64, error)
}

// Free implements Freer: the bytes an unprivileged process may still write.
func (OS) Free(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
