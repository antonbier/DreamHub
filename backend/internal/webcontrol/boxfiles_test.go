package webcontrol

import (
	"reflect"
	"testing"
)

func TestParseFileList(t *testing.T) {
	cases := map[string][]string{
		`{"result":true,"files":["/etc/enigma2/netcastserver_b.json","netcastserver_a.json","/etc/enigma2/x.txt"]}`: {"netcastserver_a.json", "netcastserver_b.json"},
		`{"files":[{"path":"/etc/enigma2/netcastserver_c.json"},{"name":"netcastserver_d.json"},{"size":3}]}`:       {"netcastserver_c.json", "netcastserver_d.json"},
		`{"files":[]}`: nil,
	}
	for in, want := range cases {
		got, err := ParseFileList([]byte(in), "netcastserver_*.json")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v %v, want %v", in, got, err, want)
		}
	}
	if _, err := ParseFileList([]byte("<xml/>"), "*"); err == nil {
		t.Error("xml accepted")
	}
}
