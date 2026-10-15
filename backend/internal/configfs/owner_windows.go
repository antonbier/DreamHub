//go:build windows

package configfs

import "io/fs"

func fileOwner(fs.FileInfo) (uid, gid int) { return 0, 0 }
