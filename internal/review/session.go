package review

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/record"
)

// Summary counts what a session did.
type Summary struct {
	Confirmed, Rejected, Skipped int
	// Findings is how many findings the verdicts reached, which is more than the problems decided
	// when a problem was seen in several runs.
	Findings int
	Quit     bool
}

// Session walks the pending problems one at a time.
type Session struct {
	In       *bufio.Reader
	Out      io.Writer
	Reviewer string
	Now      func() time.Time
}

const menu = "[c]onfirm  [r]eject  [e]dit severity or title  [s]kip  [q]uit > "

// Run shows each problem that has a pending finding and applies the reviewer's key. End of input
// counts as quit. Nothing is written for a problem until a verdict is chosen.
func (s *Session) Run(set *Set) (Summary, error) {
	var sum Summary
	groups := set.Groups(Pending)
	if len(groups) == 0 {
		fmt.Fprintln(s.Out, "No pending findings.")
		return sum, nil
	}
	fmt.Fprintf(s.Out, "%d problems to review, reviewer %s. Type a key and press Enter.\n", len(groups), s.Reviewer)
	for i, g := range groups {
		fmt.Fprintf(s.Out, "\n--- %d of %d ---\n", i+1, len(groups))
		Render(s.Out, g)
		if err := s.decide(g, &sum); err != nil {
			return sum, err
		}
		if sum.Quit {
			break
		}
	}
	fmt.Fprintf(s.Out, "\nConfirmed %d, rejected %d, skipped %d. %s changed.\n", sum.Confirmed, sum.Rejected, sum.Skipped, plural(sum.Findings, "finding"))
	return sum, nil
}

// ask prints a prompt and returns the trimmed answer. ok is false at end of input.
func (s *Session) ask(prompt string) (string, bool) {
	fmt.Fprint(s.Out, prompt)
	line, err := s.In.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(s.Out)
		return "", false
	}
	return strings.TrimSpace(line), true
}

func (s *Session) decide(g *Group, sum *Summary) error {
	var edit Decision
	for {
		in, ok := s.ask(menu)
		if !ok {
			sum.Quit = true
			return nil
		}
		switch strings.ToLower(in) {
		case "c", "confirm":
			return s.apply(g, Decision{Verdict: Confirmed, Severity: edit.Severity, Title: edit.Title}, sum)
		case "r", "reject":
			reason, ok := s.ask("Reason for rejecting (empty cancels): ")
			if !ok {
				sum.Quit = true
				return nil
			}
			if reason == "" {
				fmt.Fprintln(s.Out, "Not rejected.")
				continue
			}
			return s.apply(g, Decision{Verdict: Rejected, Reason: reason, Severity: edit.Severity, Title: edit.Title}, sum)
		case "e", "edit":
			sev, ok := s.ask(fmt.Sprintf("Severity (%s, Enter keeps %s): ", strings.Join(record.Severities, " "), g.Lead().Finding.Severity))
			if !ok {
				sum.Quit = true
				return nil
			}
			if sev != "" && !contains(record.Severities, sev) {
				fmt.Fprintf(s.Out, "%q is not a severity. Nothing changed.\n", sev)
				continue
			}
			title, ok := s.ask("Title (Enter keeps it): ")
			if !ok {
				sum.Quit = true
				return nil
			}
			edit.Severity, edit.Title = sev, clean(title)
			fmt.Fprintln(s.Out, "The edit is applied with your next confirm or reject.")
		case "s", "skip":
			sum.Skipped++
			return nil
		case "q", "quit":
			sum.Quit = true
			return nil
		default:
			fmt.Fprintf(s.Out, "%q is not a key.\n", in)
		}
	}
}

func (s *Session) apply(g *Group, d Decision, sum *Summary) error {
	d.Reviewer = s.Reviewer
	if s.Now != nil {
		d.At = s.Now()
	}
	applied, err := Apply(g, d)
	sum.Findings += len(applied)
	if len(applied) > 0 {
		if d.Verdict == Confirmed {
			sum.Confirmed++
		} else {
			sum.Rejected++
		}
		fmt.Fprintf(s.Out, "%s: %s.\n", d.Verdict, Describe(applied))
	}
	return err
}

// Describe says how far a decision reached, such as "2 findings in 2 runs".
func Describe(applied []Applied) string {
	runs := map[string]bool{}
	for _, x := range applied {
		runs[x.Run] = true
	}
	return fmt.Sprintf("%s in %s", plural(len(applied), "finding"), plural(len(runs), "run"))
}
