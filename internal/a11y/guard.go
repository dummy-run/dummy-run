package a11y

import (
	"context"

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
