package main

import (
	"strings"
	"testing"
)

// TestAllNamesEveryCheck keeps --all honest: every check flag in the help is one --all turns on.
func TestAllNamesEveryCheck(t *testing.T) {
	for _, flag := range []string{"--consent", "--engines", "--image-contrast", "--keyboard", "--layout", "--markup", "--plain-language"} {
		i := strings.Index(a11yUsage, "\n  "+flag+" ")
		if i < 0 {
			t.Fatalf("%s is not in the help", flag)
		}
		if line := a11yUsage[i+1 : i+1+strings.Index(a11yUsage[i+1:], "\n")]; !strings.Contains(line, " also ") {
			t.Errorf("%s does not start with \"also\": %q", flag, line)
		}
	}
	if !strings.Contains(a11yUsage, "--all             run every check below") {
		t.Error("--all is not described")
	}
}
