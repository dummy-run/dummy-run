package mcpserver

import "testing"

func budgetServer() *Server {
	return &Server{cfg: Config{Limits: Limits{DefaultTokens: 400, MaxTokens: 400, SessionTokens: 1000, MaxRuns: 10}}}
}

// Calls that start together must not together be promised more than the session has.
func TestParallelCallsCannotPassTheSessionCap(t *testing.T) {
	s := budgetServer()
	var granted []int
	for i := 0; i < 4; i++ {
		n, err := s.reserveRun(0)
		if err != nil {
			break
		}
		granted = append(granted, n)
	}
	total := 0
	for _, n := range granted {
		total += n
	}
	if total > 1000 {
		t.Errorf("reserved %d tokens from a session of 1000: %v", total, granted)
	}
	if len(granted) != 3 || granted[2] != 200 {
		t.Errorf("granted %v, want 400, 400 and 200", granted)
	}
}

func TestSpendReleasesTheReservation(t *testing.T) {
	s := budgetServer()
	n, _ := s.reserveRun(0)
	if used, left := s.spend(n, 150); used != 150 || left != 850 {
		t.Errorf("after a run that used 150: used %d, left %d", used, left)
	}
	// A call that ended before it spent anything gives its whole reservation back.
	n, _ = s.reserveRun(0)
	if _, left := s.spend(n, 0); left != 850 {
		t.Errorf("left %d after a call that spent nothing, want 850", left)
	}
}
