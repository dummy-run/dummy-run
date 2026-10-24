package page

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// guidepup's virtual screen reader turns the page into the phrases a screen reader speaks. It is a pinned upstream
// build; see docs/TOOLS.md and NOTICE.
//
//go:embed screenreader/vsr.min.js
var vsrJS string

// VirtualScreenReaderVersion is the version of the bundled virtual screen reader.
const VirtualScreenReaderVersion = "0.33.0"

// SpokenMax is the most phrases one reading keeps, and SpokenBudget the most time it takes.
const (
	SpokenMax    = 400
	SpokenBudget = 8 * time.Second
)

// readAloudJS moves the virtual screen reader from the top of the page to the end, one item at a time, and returns
// what it said. Called as a function expression with (max, budgetMs).
const readAloudJS = `async (max, budgetMs) => {
  const v = globalThis.dummyrunVSR;
  await v.start({ container: document.body, displayCursor: false });
  const started = Date.now();
  let complete = false;
  try {
    for (let i = 0; i < max && Date.now() - started < budgetMs; i++) {
      await v.next();
      // After the last item the reader says "end of document", then wraps round to the top.
      if ((await v.lastSpokenPhrase()) === "end of document") {
        complete = true;
        break;
      }
    }
    return { phrases: (await v.spokenPhraseLog()).slice(0, max), complete };
  } finally {
    await v.stop();
  }
}`

// Spoken is what the virtual screen reader said, reading the page from top to bottom.
type Spoken struct {
	Phrases []string `json:"phrases"`
	// Complete says the reader reached the end of the page. False means it stopped at SpokenMax or SpokenBudget.
	Complete bool `json:"complete"`
}

// ReadAloud reads the loaded page with the virtual screen reader. It runs in the page's own world: in Firefox a sandbox
// wraps the DOM in a way the reader cannot read through.
func ReadAloud(ctx context.Context, conn *bidi.Conn, cx string) (*Spoken, error) {
	// The bundle is the body of the call, not an eval, so a strict content security policy cannot block it.
	load := "() => { if (!globalThis.dummyrunVSR) {\n" + vsrJS + "\n} return !!globalThis.dummyrunVSR; }"
	var loaded bool
	if err := conn.CallFunctionJSON(ctx, cx, load, &loaded); err != nil {
		return nil, fmt.Errorf("screen reader: load: %w", err)
	}
	if !loaded {
		return nil, fmt.Errorf("screen reader: the bundle did not load")
	}
	var out Spoken
	if err := conn.CallFunctionJSON(ctx, cx, readAloudJS, &out, SpokenMax, int(SpokenBudget.Milliseconds())); err != nil {
		return nil, fmt.Errorf("screen reader: %w", err)
	}
	return &out, nil
}
