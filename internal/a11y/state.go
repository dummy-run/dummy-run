package a11y

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// maxStateChars bounds the page description the state hash reads, in the UTF-16 code units JavaScript counts. A
// page past it is hashed from its start, and the state says partial.
var maxStateChars = 8_000_000

// readPageState reads the state. Tests replace it to make the read fail.
var readPageState = readState

// stateJS describes the page as the rules see it: each element's tag, its role, its ARIA attributes, and the
// attributes and states that change what assistive technology gets, with the text between them, in document order,
// open shadow roots included. It leaves out what changes on every load without changing the page: ids and the
// attributes that point at them, such as for and aria-labelledby, which it records only as present, links'
// addresses, form values, a textarea's text among them, scripts and styles. Each attribute's name and value are
// JSON-quoted, so one value can't pass for another attribute. A description past the limit keeps the part of the
// last piece that fits. It runs in a sandbox, so the
// page's own scripts can't change what it reads.
const stateJS = `(max) => {
  const skip = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "TEMPLATE"]);
  const idRefs = new Set(["aria-activedescendant", "aria-controls", "aria-describedby", "aria-details", "aria-errormessage", "aria-flowto", "aria-labelledby", "aria-owns", "for", "headers"]);
  const keep = ["alt", "title", "type", "lang", "hidden", "inert", "tabindex", "scope", "colspan", "rowspan", "autocomplete", "placeholder"];
  const out = [];
  let size = 0, elements = 0, partial = false;
  const push = (s) => {
    if (partial) return;
    if (size + s.length > max) { out.push(s.slice(0, Math.max(max - size, 0))); partial = true; return; }
    out.push(s);
    size += s.length + 1;
  };
  const walk = (node) => {
    for (let n = node.firstChild; n && !partial; n = n.nextSibling) {
      if (n.nodeType === 3) {
        const t = n.data.replace(/\s+/g, " ").trim();
        if (t) push("#" + t);
        continue;
      }
      if (n.nodeType !== 1 || skip.has(n.tagName)) continue;
      elements++;
      const attrs = [];
      for (const a of Array.from(n.attributes).map(a => a.name).sort()) {
        if (idRefs.has(a)) attrs.push(JSON.stringify(a));
        else if (a === "role" || keep.includes(a) || a.startsWith("aria-")) attrs.push(JSON.stringify(a) + "=" + JSON.stringify(n.getAttribute(a)));
      }
      if (n.disabled) attrs.push(":disabled");
      if (n.checked) attrs.push(":checked");
      if (n.open) attrs.push(":open");
      push("<" + n.tagName.toLowerCase() + (attrs.length ? " " + attrs.join(" ") : ""));
      if (n.shadowRoot) walk(n.shadowRoot);
      // A textarea's text is its starting value, a form value like any other.
      if (n.tagName !== "TEXTAREA") walk(n);
      push(">");
    }
  };
  if (document.documentElement) walk(document);
  return {text: out.join("\n"), elements, partial};
}`

// PageState identifies the page's structure and text just before the rules run. Two results with the same Hash saw
// the same elements, roles, ARIA, states and text, whatever else differed, such as an id or a token in a form. It
// doesn't cover styles, classes, ids or link addresses, so a change to those alone keeps the hash.
type PageState struct {
	// Hash is the SHA-256 of the page's description, in hex.
	Hash string `json:"hash"`
	// Elements counts the elements described.
	Elements int `json:"elements"`
	// Partial says the description passed 8 million characters, and the hash covers its start.
	Partial bool `json:"partial,omitempty"`
}

// readState hashes the page as it stands, just before the rules run.
func readState(ctx context.Context, conn *bidi.Conn, cx string) (*PageState, error) {
	var out struct {
		Text     string `json:"text"`
		Elements int    `json:"elements"`
		Partial  bool   `json:"partial"`
	}
	if err := conn.CallFunctionJSONIn(ctx, cx, "dummyrun-state", stateJS, &out, maxStateChars); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(out.Text))
	return &PageState{Hash: hex.EncodeToString(sum[:]), Elements: out.Elements, Partial: out.Partial}, nil
}
