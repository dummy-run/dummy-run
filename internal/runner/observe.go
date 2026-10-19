package runner

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

// Step is one executed tool call with its observation.
type Step struct {
	N   int
	At  time.Time
	Obs page.Observation
	// PageChanged is false when the page looked the same before and after.
	PageChanged bool
	// Refusal is set when the persona may not use the tool: it says why.
	// The browser did nothing for this step.
	Refusal string
}

// Render writes the observation the way the persona reads it.
func Render(s Step) string {
	o := s.Obs
	var b strings.Builder
	fmt.Fprintf(&b, "step %d · %s", s.N, o.Action)
	if o.Target != "" {
		fmt.Fprintf(&b, " %s", o.Target)
	}
	fmt.Fprintf(&b, " · %d ms · %s\n", o.Duration.Milliseconds(), o.URL)
	switch {
	case s.Refusal != "":
		fmt.Fprintf(&b, "REFUSED: %s\n", s.Refusal)
	case o.Error != "":
		fmt.Fprintf(&b, "ACTION FAILED: %s\n", o.Error)
	}
	if !s.PageChanged && o.Error == "" && s.N > 1 && isTry(o.Action) {
		b.WriteString("NOTE: nothing on the page changed after this action.\n")
	}
	if o.Snapshot == nil {
		b.WriteString("(the page could not be read)\n")
		return b.String()
	}
	p := o.Snapshot
	fmt.Fprintf(&b, "title: %s\n", p.Title)
	if o.Status != 0 {
		fmt.Fprintf(&b, "status: %d\n", o.Status)
	}
	if len(p.Headings) > 0 {
		fmt.Fprintf(&b, "headings: %s\n", strings.Join(p.Headings, " | "))
	}
	if len(p.Alerts) > 0 {
		fmt.Fprintf(&b, "alerts: %s\n", strings.Join(p.Alerts, " | "))
	}
	if p.Zoom > 100 && len(p.Viewport) == 2 {
		fmt.Fprintf(&b, "zoom: %d%% (the page lays out in %d x %d CSS px, device pixel ratio %g)\n", p.Zoom, p.Viewport[0], p.Viewport[1], p.DevicePixelRatio)
	}
	if len(p.Scroll) == 2 && p.Scroll[1] > 0 && len(p.Viewport) == 2 {
		fmt.Fprintf(&b, "scroll: %d of %d px (viewport %d px tall)\n", p.Scroll[0], p.Scroll[1], p.Viewport[1])
	}
	b.WriteString(problems(o))
	b.WriteString(focusLine(p))
	if p.Tree != "" {
		fmt.Fprintf(&b, "accessibility tree (roles and names computed from the page; @n matches the controls):\n%s\n", indent(p.Tree))
	}
	b.WriteString("controls:\n")
	b.WriteString(controlRows(p.Controls))
	b.WriteString("text:\n")
	for i, line := range strings.Split(p.Text, "\n") {
		if i >= 60 {
			b.WriteString("  ...\n")
			break
		}
		fmt.Fprintf(&b, "  %s\n", strings.TrimRight(line, " "))
	}
	return b.String()
}

// focusLine says which element has keyboard focus.
func focusLine(p *page.Snapshot) string {
	if p.TreeError != "" {
		return "focus: unknown (the accessibility view could not be read)\n"
	}
	f := p.Focus
	if f == nil {
		return "focus: nothing (no element has keyboard focus)\n"
	}
	line := "focus: " + f.Role
	if f.Name != "" {
		line += fmt.Sprintf(" %q", f.Name)
	} else {
		line += " (no accessible name)"
	}
	if f.Ref != "" {
		line += " @" + f.Ref
	}
	return line + "\n"
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func problems(o page.Observation) string {
	var parts []string
	if len(o.ConsoleErrors) > 0 {
		parts = append(parts, "console errors: "+strings.Join(o.ConsoleErrors, "; "))
	}
	if len(o.FailedRequests) > 0 {
		parts = append(parts, "failed requests: "+strings.Join(o.FailedRequests, "; "))
	}
	if len(o.Blocked) > 0 {
		parts = append(parts, "blocked by policy: "+strings.Join(o.Blocked, "; "))
	}
	if o.A11y != nil {
		if len(o.A11y) == 0 {
			parts = append(parts, "accessibility: no violations")
		} else {
			vs := make([]string, 0, len(o.A11y))
			for _, v := range o.A11y {
				vs = append(vs, fmt.Sprintf("%s %s: %s (%d)", v.Impact, v.ID, v.Help, v.Nodes))
			}
			parts = append(parts, "accessibility: "+strings.Join(vs, "; "))
		}
	}
	if len(parts) == 0 {
		return "problems: none\n"
	}
	return "problems: " + strings.Join(parts, " || ") + "\n"
}

func controlRows(controls []page.Control) string {
	if len(controls) == 0 {
		return "  (none)\n"
	}
	var b strings.Builder
	for i := 0; i < len(controls); i += 4 {
		end := min(i+4, len(controls))
		cells := make([]string, 0, 4)
		for _, c := range controls[i:end] {
			kind := c.Tag
			if c.Type != "" && c.Type != c.Tag {
				kind = c.Tag + "/" + c.Type
			}
			cell := fmt.Sprintf("@%s %s %q", c.Ref, kind, c.Label)
			var extra []string
			if c.Disabled {
				extra = append(extra, "disabled")
			}
			if c.Offscreen {
				extra = append(extra, "off screen")
			}
			if len(extra) > 0 {
				cell += " [" + strings.Join(extra, ",") + "]"
			}
			cells = append(cells, cell)
		}
		fmt.Fprintf(&b, "  %s\n", strings.Join(cells, "   "))
	}
	return b.String()
}

// Summary is the one-line form of an older step, so the history stays short.
func Summary(s Step) string {
	o := s.Obs
	parts := []string{fmt.Sprintf("step %d: %s %s", s.N, o.Action, o.Target), "now at " + o.URL}
	if o.Snapshot != nil && o.Snapshot.Title != "" {
		parts = append(parts, o.Snapshot.Title)
	}
	if o.Error != "" {
		parts = append(parts, "error: "+o.Error)
	}
	if len(o.Blocked) > 0 {
		parts = append(parts, fmt.Sprintf("%d request(s) blocked by policy", len(o.Blocked)))
	}
	return strings.Join(parts, " · ") + " (older step, details shortened)"
}

// Signature hashes what the persona can see, to tell whether an action changed anything.
func Signature(o page.Observation) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(o.URL))
	_, _ = h.Write([]byte(o.Error))
	if o.Snapshot != nil {
		_, _ = h.Write([]byte(o.Snapshot.Title))
		_, _ = h.Write([]byte(o.Snapshot.LogText))
		for _, c := range o.Snapshot.Controls {
			_, _ = h.Write([]byte(c.Tag + c.Type + c.Label))
		}
		// Moving focus with Tab changes what the persona sees even when nothing else does.
		if f := o.Snapshot.Focus; f != nil {
			_, _ = h.Write([]byte("focus:" + f.Ref + f.Role + f.Name))
		}
		_, _ = h.Write([]byte("tree-error:" + o.Snapshot.TreeError))
		if len(o.Snapshot.Scroll) == 2 {
			fmt.Fprintf(h, "%d", o.Snapshot.Scroll[0])
		}
	}
	return h.Sum64()
}
