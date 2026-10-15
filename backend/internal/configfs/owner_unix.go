//go:build !windows

package configfs

import (
	"io/fs"
	"syscall"
)

func fileOwner(info fs.FileInfo) (uid, gid int) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return 0, 0
}
