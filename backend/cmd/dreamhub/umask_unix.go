//go:build unix

package main

import "syscall"

// restrictFileModes makes every file and directory DreamHub creates —
// including SQLite's -wal/-shm side files, which hold recently written
// settings such as API keys — readable only by DreamHub's own user
// (PRINCIPLES.md, "Settings & secrets"). Found on the box: with the
// default umask 022 the data directory and the side files came out
// world-readable although the database itself is 0600.
func restrictFileModes() { syscall.Umask(0o077) }
