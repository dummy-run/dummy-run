package runner

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/dummy-run/dummy-run/internal/page"
)

// Budget rules. A scenario may ask for more steps on a "Budget: N steps" line, up
// to the cap, so a long but real task is not recorded as exhausted only because
// it has many parts.
const (
	DefaultStepBudget = 25
	MaxStepBudget     = 40
	// Tries is how many failed attempts at the same thing a persona makes before it gives up.
	Tries = 3
)

// StepBudget clamps a scenario's requested budget. A zero request means the default.
func StepBudget(requested int) int {
	if requested <= 0 {
		return DefaultStepBudget
	}
	return min(max(requested, DefaultStepBudget), MaxStepBudget)
}

// maxEarlyFinishes is how many times finish is sent back for coming before the scenario's minimum.
// After that it is accepted, so a model that insists cannot loop until the budget is gone.
const maxEarlyFinishes = 3

// MinSteps clamps a scenario's "Min steps" request. It leaves at least three steps of the budget
// for the work after the minimum, so a minimum can never use the whole budget.
func MinSteps(requested, budget int) int {
	if requested <= 0 {
		return 0
	}
	return min(requested, max(budget-3, 0))
}

// earlyFinish answers a finish call that comes before the scenario's minimum, or returns "" to accept it.
// taken is the number of steps the persona took itself. Finish is accepted when the verdict is blocked
// (the persona really cannot go on), when the engine has forced the finish or the persona has given up,
// and after maxEarlyFinishes refusals.
func earlyFinish(minSteps, taken, refused int, forced bool, args json.RawMessage) string {
	if minSteps <= 0 || taken >= minSteps || forced || refused >= maxEarlyFinishes {
		return ""
	}
	var v struct {
		Verdict string `json:"verdict"`
	}
	_ = json.Unmarshal(args, &v)
	if v.Verdict == "blocked" {
		return ""
	}
	return fmt.Sprintf("Not finished yet: you have taken %d of at least %d actions. Carry on with the scenario: open another page, read it and try the next thing. "+
		"Call finish early only with the verdict blocked, when the site really stops you from going on.", max(taken, 0), minSteps)
}

var tryActions = map[string]bool{"click": true, "fill": true, "type": true, "select": true, "visit": true}

func isTry(action string) bool { return tryActions[action] }

// GaveUp records why the persona stopped.
type GaveUp struct {
	Step   int
	Action string
	Tries  int
}

// Patience counts failed tries at the same thing. A try is an action that
// should change the page; it failed when it raised an error or left the page
// as it was. The same thing is the same action on the same control at the same
// address, the control known by its tag and label since pages renumber them.
type Patience struct {
	failed map[string]int
	gaveUp *GaveUp
}

// NewPatience starts counting.
func NewPatience() *Patience { return &Patience{failed: map[string]int{}} }

var refPattern = regexp.MustCompile(`@(\d+)\b`)

// Note records one step. controls are the numbered controls the persona saw
// before it acted, so "@12" can be named.
func (p *Patience) Note(step int, url, action, target string, controls []page.Control, failed bool) {
	if !failed || !isTry(action) {
		return
	}
	thing := action + " " + Describe(target, controls)
	key := url + "\x00" + thing
	p.failed[key]++
	if p.failed[key] >= Tries && p.gaveUp == nil {
		p.gaveUp = &GaveUp{Step: step, Action: thing, Tries: p.failed[key]}
	}
}

// Describe replaces "@12" in a target with the control's tag and label.
func Describe(target string, controls []page.Control) string {
	return refPattern.ReplaceAllStringFunc(target, func(ref string) string {
		for _, c := range controls {
			if "@"+c.Ref == ref {
				return fmt.Sprintf("%s %q", c.Tag, c.Label)
			}
		}
		return ref
	})
}

// Spent reports whether the persona has given up.
func (p *Patience) Spent() bool { return p.gaveUp != nil }

// GaveUp returns why, or nil.
func (p *Patience) GaveUp() *GaveUp { return p.gaveUp }
