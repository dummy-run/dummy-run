package mcpserver

import "fmt"

// countSweep counts one a11y_sweep call against the session.
func (s *Server) countSweep() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sweeps >= s.cfg.Limits.MaxSweeps {
		return fmt.Errorf("this session has made %d a11y_sweep calls, which is its limit. Start a new session to sweep more", s.sweeps)
	}
	s.sweeps++
	return nil
}

// reserveRun counts one run_persona call and returns the token budget it may spend: the smaller of the
// request, the per-call cap and what is left of the session after earlier runs and other calls that hold
// a reservation. The tokens are reserved until spend releases them, so parallel calls cannot together
// pass the session cap.
func (s *Server) reserveRun(requested int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.cfg.Limits
	if s.runs >= l.MaxRuns {
		return 0, fmt.Errorf("this session has made %d run_persona calls, which is its limit. Start a new session to run more", s.runs)
	}
	left := l.SessionTokens - s.tokensUsed - s.reserved
	if left <= 0 {
		return 0, fmt.Errorf("this session has spent its %d token budget. Start a new session to run more", l.SessionTokens)
	}
	if requested <= 0 {
		requested = l.DefaultTokens
	}
	s.runs++
	granted := min(requested, l.MaxTokens, left)
	s.reserved += granted
	return granted, nil
}

// spend releases the reservation a run held and adds the tokens it used to the session. Pass used 0
// when a call ends before it spends anything.
func (s *Server) spend(reserved, used int) (total, left int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reserved = max(s.reserved-reserved, 0)
	s.tokensUsed += used
	return s.tokensUsed, max(s.cfg.Limits.SessionTokens-s.tokensUsed-s.reserved, 0)
}
