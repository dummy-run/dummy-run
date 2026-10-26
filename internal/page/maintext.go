package page

import (
	"context"
	"fmt"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// MainHTMLMax is the most characters of main content one page gives the plain language check.
const MainHTMLMax = 200_000

// mainHTMLJS returns the page's main content as HTML: the first main landmark that is displayed, or the body without its header, navigation,
// footer and asides. Scripts, styles, hidden elements and inline images are dropped, so only words a visitor reads
// are left. An element the page's CSS hides with display, visibility or opacity is dropped too. Called with (max).
const mainHTMLJS = `(max) => {
  // A page can keep more than one main element and hide all but one. The first one that shows is used: one that is
  // displayed, with opacity and visibility, or one with no box of its own whose children show. Failing that, the
  // first one that is displayed at all, then the first one.
  const mains = [...document.querySelectorAll("main, [role=main]")];
  const boxless = (el) => getComputedStyle(el).display === "contents";
  const displayed = (el) => !el.checkVisibility || el.checkVisibility() || boxless(el);
  const shows = (el) => !el.checkVisibility ||
    (boxless(el) ? [...el.children].some(shows) : el.checkVisibility({ opacityProperty: true, visibilityProperty: true }));
  const root = mains.find(shows) || mains.find(displayed) || mains[0] || document.body;
  if (!root) return { html: "", selector: "", truncated: false };
  const copy = root.cloneNode(true);
  // The copy has no styles, so what CSS hides is found on the page and removed from the copy, which has the same
  // elements in the same order. The root is checked too, since querySelectorAll finds only what is inside it.
  const live = [root, ...root.querySelectorAll("*")], copied = [copy, ...copy.querySelectorAll("*")];
  // display: none and opacity: 0 hide everything inside, so the whole copy goes. visibility: hidden can be undone
  // by a child with visibility: visible, so only the hidden element's own text goes.
  const gone = [], silent = [];
  live.forEach((el, i) => {
    // An element with display: contents has no box of its own, so checkVisibility says no, but its children show.
    const boxless = getComputedStyle(el).display === "contents";
    if (el.checkVisibility && !boxless && !el.checkVisibility({ opacityProperty: true })) gone.push(copied[i]);
    else if (getComputedStyle(el).visibility !== "visible") silent.push(copied[i]);
  });
  for (const el of silent) for (const n of [...el.childNodes]) if (n.nodeType === Node.TEXT_NODE) n.remove();
  for (const el of gone) el.remove();
  const selector = root === document.body ? "body" : (root.tagName === "MAIN" ? "main" : "[role=main]");
  if (gone.includes(copy)) return { html: "", selector, truncated: false };
  const drop = "script, style, noscript, template, svg, canvas, iframe, [hidden], [aria-hidden=true]" +
    (root === document.body ? ", header, nav, footer, aside, [role=banner], [role=navigation], [role=contentinfo], [role=complementary]" : "");
  for (const el of copy.querySelectorAll(drop)) el.remove();
  const html = "<!doctype html><html lang=\"" + (document.documentElement.lang || "en") + "\"><body>" + copy.innerHTML + "</body></html>";
  return { html: html.slice(0, max), truncated: html.length > max, selector };
}`

// MainContent is the page's main content, ready for a prose checker.
type MainContent struct {
	HTML string `json:"html"`
	// Truncated says the content was longer than MainHTMLMax, so only its start is in HTML.
	Truncated bool `json:"truncated"`
	// Selector names the element the content came from: main, [role=main] or body.
	Selector string `json:"selector"`
}

// MainHTML reads the loaded page's main content.
func MainHTML(ctx context.Context, conn *bidi.Conn, cx string) (*MainContent, error) {
	var out MainContent
	if err := conn.CallFunctionJSON(ctx, cx, mainHTMLJS, &out, MainHTMLMax); err != nil {
		return nil, fmt.Errorf("main content: %w", err)
	}
	return &out, nil
}
