package pdfua

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// veraPDF runs only where it's installed, so these tests skip without it.
func verapdf(t *testing.T) string {
	t.Helper()
	p, err := Find()
	if err != nil {
		t.Skip("set DUMMYRUN_VERAPDF or put verapdf on the PATH to run the veraPDF tests")
	}
	return p
}

func TestCheckReportsFailedRulesAndUnreadableFiles(t *testing.T) {
	bin := verapdf(t)
	broken := filepath.Join(t.TempDir(), "broken.pdf")
	if err := os.WriteFile(broken, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := Check(ctx, bin, filepath.Join("testdata", "untagged.pdf"), broken)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 results, got %d", len(res))
	}
	untagged := res[0]
	if untagged.Compliant || untagged.Error != "" || len(untagged.Rules) == 0 {
		t.Fatalf("untagged PDF: %+v", untagged)
	}
	var fonts bool
	for _, r := range untagged.Rules {
		if r.Clause == "" || r.Description == "" || r.Checks < 1 {
			t.Errorf("incomplete rule %+v", r)
		}
		fonts = fonts || strings.HasPrefix(r.Clause, "7.21.4.1")
	}
	if !fonts {
		t.Errorf("the unembedded font (7.21.4.1) isn't reported: %+v", untagged.Rules)
	}
	if res[1].Error == "" || res[1].Compliant {
		t.Errorf("a file that isn't a PDF: %+v", res[1])
	}
}

func TestFindHonoursTheVariable(t *testing.T) {
	t.Setenv("DUMMYRUN_VERAPDF", filepath.Join(t.TempDir(), "missing"))
	if _, err := Find(); err == nil || !strings.Contains(err.Error(), "DUMMYRUN_VERAPDF") {
		t.Errorf("want a DUMMYRUN_VERAPDF error, got %v", err)
	}
}
