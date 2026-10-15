package recfs

// Freer is an FS that knows the free space where a path lives — for work
// that writes a large new file next to a recording (trimming).
type Freer interface {
	Free(path string) (uint64, error)
}
