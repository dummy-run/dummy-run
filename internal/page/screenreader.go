package page

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
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

// readAloudBody moves the virtual screen reader from the top of the page to the end, one item at a time, and returns
// what it said. It runs after the bundle, in the same call, with max and budgetMs in scope.
const readAloudBody = `
  const v = vsr;
  if (!v) throw new Error("the bundle did not load");
  const started = Date.now();
  let complete = false;
  // A step the page holds up still ends at the budget, so one slow element can't stall the observation.
  const late = Symbol("late");
  const step = (p) => {
    const left = budgetMs - (Date.now() - started);
    let timer;
    const timeout = new Promise((resolve) => { timer = setTimeout(() => resolve(late), Math.max(left, 0)); });
    return Promise.race([p, timeout]).finally(() => clearTimeout(timer));
  };
  try {
    // Inside the try, so a start that fails part-way still reaches stop. Every call to the reader runs inside the
    // budget, start and the log included, so no step of it can hold the call past the budget.
    if ((await step(v.start({ container: document.body, displayCursor: false }))) === late) return { phrases: [], complete };
    for (let i = 0; i < max && Date.now() - started < budgetMs; i++) {
      if ((await step(v.next())) === late) break;
      // After the last item the reader says "end of document", then wraps round to the top.
      const last = await step(v.lastSpokenPhrase());
      if (last === late) break;
      if (last === "end of document") {
        complete = true;
        break;
      }
    }
    const log = await step(v.spokenPhraseLog());
    if (log === late) return { phrases: [], complete: false };
    return { phrases: log.slice(0, max), complete };
  } finally {
    // stop gets a second of its own, so a reader that hangs on the way out still lets the call end.
    let timer;
    await Promise.race([Promise.resolve().then(() => v.stop()).catch(() => {}), new Promise((r) => { timer = setTimeout(r, 1000); })]);
    clearTimeout(timer);
  }
`

// readAloudJS is the bundle and the reading in one call. The bundle assigns the reader to a variable local to the
// call, never to a global, so page script can't put its own reader in its place. Being the body of the call rather
// than an eval, it is not blocked by a strict content security policy.
var readAloudJS = "async (max, budgetMs) => {\nlet vsr;\n" + localReader(vsrJS) + "\n" + readAloudBody + "}"

// localReader turns the bundle's one global assignment into an assignment to the call's own vsr variable.
func localReader(bundle string) string {
	const global = "globalThis.dummyrunVSR="
	if strings.Count(bundle, global) != 1 {
		panic("screen reader: the bundle should assign " + global + " exactly once; see screenreader/build/entry.mjs")
	}
	return strings.Replace(bundle, global, "vsr=", 1)
}

// Spoken is what the virtual screen reader said, reading the page from top to bottom.
type Spoken struct {
	Phrases []string `json:"phrases"`
	// Complete says the reader reached the end of the page. False means it stopped at SpokenMax or SpokenBudget.
	Complete bool `json:"complete"`
}

// ReadAloud reads the loaded page with the virtual screen reader. It runs in the page's own world: in Firefox a sandbox
// wraps the DOM in a way the reader cannot read through.
func ReadAloud(ctx context.Context, conn *bidi.Conn, cx string) (*Spoken, error) {
	// The reading bounds itself in the page. The call gets a little more, so a page that blocks its own thread still
	// can't hold the run.
	ctx, cancel := context.WithTimeout(ctx, SpokenBudget+5*time.Second)
	defer cancel()
	var out Spoken
	if err := conn.CallFunctionJSON(ctx, cx, readAloudJS, &out, SpokenMax, int(SpokenBudget.Milliseconds())); err != nil {
		return nil, fmt.Errorf("screen reader: %w", err)
	}
	return &out, nil
}
