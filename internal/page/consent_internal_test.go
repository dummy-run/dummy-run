package page

import (
	"strings"
	"testing"
)

// TestAutoconsentKeepsTheBacktrackingFix guards the one change to the bundled autoconsent: a new version must carry
// it again, or keep the pattern safe some other way.
func TestAutoconsentKeepsTheBacktrackingFix(t *testing.T) {
	if strings.Contains(autoconsentJS, "|des cookies|de cookies|") {
		t.Error("the bundle has the close-button pattern that backtracks exponentially; see docs/TOOLS.md")
	}
	if !strings.Contains(autoconsentJS, "|hier|x))*") {
		t.Error("the patched close-button pattern is missing from the bundle")
	}
}
