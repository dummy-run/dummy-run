package page

import (
	"context"
	"fmt"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// MainHTMLMax is the most characters of main content one page gives the plain language check.
const MainHTMLMax = 200_000

// mainHTMLJS returns the page's main content as HTML: the main landmark, or the body without its header, navigation,
// footer and asides. Scripts, styles, hidden elements and inline images are dropped, so only words a visitor reads
// are left. An element the page's CSS hides with display, visibility or opacity is dropped too. Called with (max).
const mainHTMLJS = `(max) => {
  const root = document.querySelector("main, [role=main]") || document.body;
  if (!root) return { html: "", selector: "", truncated: false };
  const copy = root.cloneNode(true);
  // The copy has no styles, so what CSS hides is found on the page and removed from the copy, which has the same
  // elements in the same order.
  const live = root.querySelectorAll("*"), copied = copy.querySelectorAll("*");
  const gone = [];
  live.forEach((el, i) => {
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true, opacityProperty: true })) gone.push(copied[i]);
  });
  for (const el of gone) el.remove();
  const drop = "script, style, noscript, template, svg, canvas, iframe, [hidden], [aria-hidden=true]" +
    (root === document.body ? ", header, nav, footer, aside, [role=banner], [role=navigation], [role=contentinfo], [role=complementary]" : "");
  for (const el of copy.querySelectorAll(drop)) el.remove();
  const html = "<!doctype html><html lang=\"" + (document.documentElement.lang || "en") + "\"><body>" + copy.innerHTML + "</body></html>";
  return { html: html.slice(0, max), truncated: html.length > max,
    selector: root === document.body ? "body" : (root.tagName === "MAIN" ? "main" : "[role=main]") };
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
