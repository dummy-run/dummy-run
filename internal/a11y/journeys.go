package a11y

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/journey"
	"github.com/dummy-run/dummy-run/internal/tech"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

// RuleJourneyKeyboard is a journey that works by mouse but not by keyboard.
const RuleJourneyKeyboard = "dummyrun-journey-keyboard"

// journeysTimeout bounds the journeys for one page. Each journey loads the page twice.
const journeysTimeout = 4 * time.Minute

// journeyCheck finds the page's journeys and tries each by mouse and by keyboard, in a tab of its own so the sweep's
// tab is left as it was. A journey that works by mouse but not by keyboard is a violation of 2.1.1. A journey whose
// link robots.txt disallows isn't tried: allowed says whether it may be loaded, and if not, why.
func journeyCheck(ctx context.Context, conn *bidi.Conn, cfg Config, u string, vp Viewport, res *PageResult, allowed func(string) (bool, string)) {
	jctx, cancel := context.WithTimeout(ctx, journeysTimeout)
	defer cancel()
	cx, err := conn.CreateContext(jctx)
	if err != nil {
		res.JourneysError = firstLine(err.Error())
		return
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = conn.CloseContext(cctx, cx)
	}()
	if err := conn.SetViewport(jctx, cx, vp.Width, vp.Height); err != nil {
		res.JourneysError = firstLine(err.Error())
		return
	}
	// The tab sends nothing but GET and HEAD, and with --respect-robots loads no page robots.txt disallows.
	d := &journey.Driver{Conn: conn, CX: cx}
	unguard, err := d.Guard(jctx, func(raw string) bool {
		ok, _ := allowed(raw)
		return ok
	})
	if err != nil {
		res.JourneysError = firstLine(err.Error())
		return
	}
	defer unguard()
	if cfg.Identify {
		v, err := conn.Evaluate(jctx, cx, "navigator.userAgent")
		if err == nil {
			ua, _ := v.String()
			_ = conn.SetUserAgentOverride(jctx, telemetry.UserAgent(ua, true), []string{cx})
		}
	}
	if _, err := conn.Navigate(jctx, cx, u); err != nil {
		res.JourneysError = firstLine(err.Error())
		return
	}
	prints := tech.Own
	if extra, err := tech.FromEnv(); err == nil && len(extra) > 0 {
		prints = append(slices.Clone(tech.Own), extra...)
	}
	found, _ := tech.Detect(jctx, conn, cx, prints)
	js, err := d.Find(jctx, u, found)
	if err != nil {
		res.JourneysError = firstLine(err.Error())
		return
	}
	f := Finding{Rule: RuleJourneyKeyboard, Impact: "serious", WCAG: []string{"2.1.1"},
		Help:    "A journey works by mouse but not by keyboard",
		HelpURL: "https://www.w3.org/WAI/WCAG22/Understanding/keyboard.html", Nodes: []Node{}}
	path := pagePath(u)
	for i := range js {
		if js[i].Href != "" && js[i].Mouse == nil {
			if ok, why := allowed(js[i].Href); !ok {
				js[i].Mouse = &journey.Verdict{Outcome: journey.Skipped, Detail: why}
				js[i].Keyboard = &journey.Verdict{Outcome: journey.Skipped, Detail: why}
				continue
			}
		}
		d.Try(jctx, u, &js[i])
		j := js[i]
		if !j.KeyboardShortfall() {
			continue
		}
		detail := fmt.Sprintf("%s (%q) works by mouse; by keyboard it's %s", j.Kind, j.Label, j.Keyboard.Outcome)
		if j.Keyboard.Detail != "" {
			detail += ": " + j.Keyboard.Detail
		}
		n := Node{Target: j.Selector, Detail: detail}
		if e := cfg.Allow.Match(RuleJourneyKeyboard, path, n.Target); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		f.Nodes = append(f.Nodes, n)
	}
	res.Journeys = js
	if len(f.Nodes) > 0 {
		res.Violations = append(res.Violations, f)
	}
}
