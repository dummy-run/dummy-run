package a11y

import (
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/page"
)

// The optional checks that couldn't run say so in the terminal summary, without control characters a page or a tool
// could put in the message.
func TestSummaryListsOtherCheckErrors(t *testing.T) {
	rep := &Report{Browser: "chrome", Results: []PageResult{
		{URL: "https://example.com/a", Browser: "chrome", Viewport: "desktop", Scheme: "light",
			PlainLanguageError: "vale not found", HTMLError: "vnu\u001b[2K failed", LighthouseError: "needs Chrome",
			Consent: &page.Consent{Error: "timed out"}, EngineErrors: map[string]string{"qualweb": "boom\u0007"}},
		{URL: "https://example.com/b", Browser: "chrome", Viewport: "desktop", Scheme: "light"},
	}}
	rep.summarise(Config{Allow: &AllowList{}})
	var b strings.Builder
	rep.WriteSummary(&b)
	out := b.String()
	for _, want := range []string{
		"PLAIN https://example.com/a (chrome/desktop/light): vale not found",
		"HTML https://example.com/a (chrome/desktop/light): vnu[2K failed",
		"LIGHTHOUSE https://example.com/a (chrome/desktop/light): needs Chrome",
		"CONSENT https://example.com/a (chrome/desktop/light): timed out",
		"other checks failed on 1 runs",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
	if strings.ContainsAny(out, "\u001b\u0007") {
		t.Errorf("summary has control characters: %q", out)
	}
}
