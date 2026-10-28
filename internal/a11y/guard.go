package a11y

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/journey"
)

// continueWith sends on a request a tab's guard lets through. With preview access it adds the access headers,
// since a request two interceptions hold is left to the tab's guard: see access.Attach.
func continueWith(conn *bidi.Conn, cfg Config) func(context.Context, bidi.BeforeRequestSent) error {
	return func(ctx context.Context, e bidi.BeforeRequestSent) error {
		return cfg.Access.Continue(ctx, conn, e)
	}
}

// guardTab makes the sweep's own tab read-only for as long as the sweep runs. The keyboard and mouse comparison,
// the repeated clicks and the other checks that press the page's controls run the page's own scripts, and a script
// can send a form or call an API. A request other than GET or HEAD fails before it leaves the browser, and so does a
// page load robots.txt disallows, with --respect-robots. The returned driver counts what it stopped.
func guardTab(ctx context.Context, conn *bidi.Conn, cx string, cfg Config, allowed func(string) (bool, string)) (*journey.Driver, func(), error) {
	d := &journey.Driver{Conn: conn, CX: cx, Continue: continueWith(conn, cfg)}
	stop, err := d.Guard(ctx, func(raw string) bool {
		ok, _ := allowed(raw)
		return ok
	})
	if err != nil {
		return nil, nil, err
	}
	return d, stop, nil
}

// openedTabs guards the tabs the page opens itself, with window.open or a link to a new window. They aren't tabs
// the sweep made, so no tab guard holds their requests: the same rules apply to them here, and each such tab is
// closed as soon as it opens, so it can't take focus from the page under test.
type openedTabs struct {
	stopped atomic.Int32
	// opened counts the tabs pages opened, each closed as soon as it appeared.
	opened atomic.Int32
	// pending counts the decisions and closes still under way, so a page's count can wait for them.
	pending atomic.Int32
	off     []func()
}

// guardOpenedTabs holds every request in the browser and answers the ones no other guard holds. With preview access
// those are held by the access interception too, which leaves them to this one: see access.Attach.
func guardOpenedTabs(ctx context.Context, conn *bidi.Conn, cfg Config, allowed func(string) (bool, string)) (*openedTabs, error) {
	g := &openedTabs{}
	var id atomic.Value
	id.Store("")
	g.off = append(g.off, conn.OnBeforeRequestSent(func(e bidi.BeforeRequestSent) {
		mine := id.Load().(string)
		if !e.IsBlocked || mine == "" || !slices.Contains(e.Intercepts, mine) {
			return
		}
		for _, in := range e.Intercepts {
			if in != mine && !cfg.Access.Owns(in) {
				return
			}
		}
		// A write is counted here, before anything else happens, so a check that reads the count as soon as it
		// finishes, as Lighthouse's does, never misses one.
		m := strings.ToUpper(e.Request.Method)
		write := m != "GET" && m != "HEAD"
		if write {
			g.stopped.Add(1)
		}
		g.pending.Add(1)
		go func() {
			defer g.pending.Add(-1)
			ok := !write
			if ok && e.Navigation != "" {
				if ok, _ = allowed(e.Request.URL); !ok {
					g.stopped.Add(1)
				}
			}
			// The command's time starts after the decision, since the robots check can wait on another origin.
			cctx, cancel := context.WithTimeout(context.Background(), bidi.DefaultTimeout)
			defer cancel()
			if !ok {
				_ = conn.FailRequest(cctx, e.Request.Request)
				return
			}
			_ = cfg.Access.Continue(cctx, conn, e)
		}()
	}))
	g.off = append(g.off, conn.OnEvent("browsingContext.contextCreated", func(_ string, params json.RawMessage) {
		var c struct {
			Context        string `json:"context"`
			Parent         string `json:"parent"`
			OriginalOpener string `json:"originalOpener"`
		}
		if json.Unmarshal(params, &c) != nil || c.Parent != "" || c.OriginalOpener == "" {
			return
		}
		g.opened.Add(1)
		g.pending.Add(1)
		go func() {
			defer g.pending.Add(-1)
			cctx, cancel := context.WithTimeout(context.Background(), bidi.DefaultTimeout)
			defer cancel()
			_ = conn.CloseContext(cctx, c.Context)
		}()
	}))
	if err := conn.Subscribe(ctx, []string{"network.beforeRequestSent", "browsingContext.contextCreated"}); err != nil {
		g.close()
		return nil, err
	}
	in, err := conn.AddInterceptPhases(ctx, nil, "beforeRequestSent")
	if err != nil {
		g.close()
		return nil, err
	}
	id.Store(in)
	return g, nil
}

// Stopped is how many requests from the tabs the page opened were failed.
func (g *openedTabs) Stopped() int {
	if g == nil {
		return 0
	}
	return int(g.stopped.Load())
}

// Opened is how many tabs pages opened.
func (g *openedTabs) Opened() int {
	if g == nil {
		return 0
	}
	return int(g.opened.Load())
}

// settle waits, for up to wait, until no decision or close is under way, so a robots refusal that a slow robots.txt
// held up is counted for the page that caused it and not the next one.
func (g *openedTabs) settle(wait time.Duration) {
	if g == nil {
		return
	}
	deadline := time.Now().Add(wait)
	for g.pending.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
}

func (g *openedTabs) close() {
	for _, off := range g.off {
		off()
	}
}
