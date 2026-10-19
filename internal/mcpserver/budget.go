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
// request, the per-call cap and what is left of the session.
func (s *Server) reserveRun(requested int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.cfg.Limits
	if s.runs >= l.MaxRuns {
		return 0, fmt.Errorf("this session has made %d run_persona calls, which is its limit. Start a new session to run more", s.runs)
	}
	left := l.SessionTokens - s.tokensUsed
	if left <= 0 {
		return 0, fmt.Errorf("this session has spent its %d token budget. Start a new session to run more", l.SessionTokens)
	}
	if requested <= 0 {
		requested = l.DefaultTokens
	}
	s.runs++
	return min(requested, l.MaxTokens, left), nil
}

// spend adds the tokens a run used to the session.
func (s *Server) spend(tokens int) (used, left int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokensUsed += tokens
	return s.tokensUsed, max(s.cfg.Limits.SessionTokens-s.tokensUsed, 0)
}
