//go:build windows

package hubs

func freeBytesAt(string) (int64, bool) { return 0, false }
