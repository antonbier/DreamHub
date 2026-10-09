package webcontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestEditorRestore_Path confirms the fix for a real-box bug found reading
// WebBouquetEditor's own source (BouquetEditor.py, confirmed live
// 2026-10-09): backup takes a bare name it sanitizes, joins with "/tmp" and
// appends ".tar" to itself, but restore takes that result as a literal
// path and only checks exists() on it — so restore must be given the same
// name backup was, reconstructed into the path backup actually wrote.
func TestEditorRestore_Path(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.FormValue("Filename")
		w.Write([]byte(`<e2simplexmlresult><e2state>True</e2state><e2statetext>ok</e2statetext></e2simplexmlresult>`))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.AllowWrites = true
	if err := c.EditorRestore(context.Background(), "dreamhub-20261009-195423-ab12"); err != nil {
		t.Fatal(err)
	}
	if want := "/tmp/dreamhub_20261009_195423_ab12.tar"; got != want {
		t.Errorf("Filename = %q, want %q", got, want)
	}
}
