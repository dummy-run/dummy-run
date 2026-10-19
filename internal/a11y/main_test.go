package a11y

import (
	"fmt"
	"os"
	"testing"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// TestMain fails the package when a test left a browser open.
func TestMain(m *testing.M) {
	code := m.Run()
	if n := bidi.OpenBrowsers(); n > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: %d browsers were launched and never closed\n", n)
		bidi.CloseAll()
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
