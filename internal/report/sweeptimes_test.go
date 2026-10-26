package report

import (
	"testing"

	"github.com/dummy-run/dummy-run/internal/a11y"
)

// TestSweepTimesCompareInstants checks sweeps written with different offsets order by when they happened, not by
// how their times read.
func TestSweepTimesCompareInstants(t *testing.T) {
	start, end := sweepTimes([]*a11y.Report{
		{StartedAt: "2026-10-26T09:00:00Z", EndedAt: "2026-10-26T09:30:00Z"},
		{StartedAt: "2026-10-26T10:00:00+02:00", EndedAt: "2026-10-26T10:15:00+02:00"},
		{StartedAt: "not a time", EndedAt: "zzz"},
		{},
	})
	if start != "2026-10-26T10:00:00+02:00" || end != "2026-10-26T09:30:00Z" {
		t.Errorf("start %q, end %q", start, end)
	}
	if s, e := sweepTimes(nil); s != "" || e != "" {
		t.Errorf("no sweeps: %q, %q", s, e)
	}
}
