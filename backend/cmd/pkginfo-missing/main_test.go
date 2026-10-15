package main

import "testing"

func TestList(t *testing.T) {
	index := []byte("Package: ffmpeg\nVersion: 1\nDescription: x\n\nPackage: enigma2-plugin-extensions-newthing\nVersion: 2\nDescription: A new thing\n\nPackage: foo\nVersion: 3\n")
	missing, total := list(index)
	if total != 3 || len(missing) != 2 {
		t.Fatalf("total=%d missing=%v", total, missing)
	}
	if missing[0].Name != "enigma2-plugin-extensions-newthing" || missing[0].Description != "A new thing" || missing[1].Name != "foo" {
		t.Errorf("missing = %+v", missing)
	}
}
