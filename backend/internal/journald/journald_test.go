package journald

import (
	"strings"
	"testing"
)

func TestParse_DecodesRealJournalctlJSONLines(t *testing.T) {
	out := strings.Join([]string{
		`{"__REALTIME_TIMESTAMP":"1774122941000000","_SYSTEMD_UNIT":"dreamhub.service","PRIORITY":"3","MESSAGE":"webcontrol: 502"}`,
		`{"__REALTIME_TIMESTAMP":"1774122942000000","_SYSTEMD_UNIT":"enigma2.service","PRIORITY":"6","MESSAGE":"tuner ok"}`,
	}, "\n") + "\n"

	entries := parse([]byte(out), "")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Unit != "dreamhub.service" || entries[0].Level != LevelErr || entries[0].Message != "webcontrol: 502" {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[0].Time.Unix() != 1774122941 {
		t.Errorf("expected unix time 1774122941, got %d", entries[0].Time.Unix())
	}
}

func TestParse_TextFilterIsCaseInsensitive(t *testing.T) {
	out := `{"__REALTIME_TIMESTAMP":"1774122941000000","_SYSTEMD_UNIT":"enigma2.service","PRIORITY":"6","MESSAGE":"Tuner Signal Weak"}` + "\n"
	if len(parse([]byte(out), "signal")) != 1 {
		t.Error("expected the entry to match a lowercase substring of an uppercase message")
	}
	if len(parse([]byte(out), "nomatch")) != 0 {
		t.Error("expected no match for unrelated text")
	}
}

func TestParse_NonUTF8MessageAndMalformedLinesDontCrash(t *testing.T) {
	out := strings.Join([]string{
		`{"__REALTIME_TIMESTAMP":"1774122941000000","_SYSTEMD_UNIT":"x","PRIORITY":"6","MESSAGE":[104,105]}`,
		`not even json`,
		`{"__REALTIME_TIMESTAMP":"1774122942000000","_SYSTEMD_UNIT":"y","PRIORITY":"6","MESSAGE":"ok"}`,
	}, "\n") + "\n"

	entries := parse([]byte(out), "")
	if len(entries) != 2 {
		t.Fatalf("expected the malformed line dropped and the byte-array message kept (as empty text), got %d entries: %+v", len(entries), entries)
	}
	if entries[0].Message != "" {
		t.Errorf("a non-UTF-8 message should decode to empty text, got %q", entries[0].Message)
	}
}
