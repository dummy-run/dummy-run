package main

import "testing"

// Asking for help is not an error, so the documented help commands exit 0.
func TestHelpExitsZero(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"run", "-h"}, {"a11y", "-h"}, {"audit", "a11y", "-h"}, {"version"}} {
		if got := run(args); got != 0 {
			t.Errorf("dummyrun %v exited %d, want 0", args, got)
		}
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{{}, {"nonsense"}, {"run"}, {"run", "--no-such-flag"}} {
		if got := run(args); got != 2 {
			t.Errorf("dummyrun %v exited %d, want 2", args, got)
		}
	}
}
