// The page side of the Tab walk. Called as a function expression with (op, arg) in a sandbox, so its state
// lives beside the page and not in it. Ops:
//   "init"   remember how each focusable element looks when it does not have focus; returns the count
//   "state"  describe the element that has focus now
//   "blur"   take focus off the element that has it, so it can be photographed without focus
//   "focus"  put focus back on the element with the id in arg
//   "skip"   describe the skip link at the first stop: does its target exist and can it be seen
(op, arg) => {
  const S = (globalThis.__drTab = globalThis.__drTab || { ids: new WeakMap(), els: [], look: new WeakMap() });
  const idOf = (el) => {
    if (!S.ids.has(el)) {
      S.ids.set(el, S.els.length);
      S.els.push(new WeakRef(el));
    }
    return S.ids.get(el);
  };
  const round = (n) => Math.round(n * 10) / 10;
  const part = (el) => {
    if (el.id && document.querySelectorAll("#" + CSS.escape(el.id)).length === 1) return "#" + CSS.escape(el.id);
    let s = el.tagName.toLowerCase();
    const cls = [...el.classList].filter((c) => /^[A-Za-z_-][\w-]*$/.test(c)).slice(0, 2);
    if (cls.length) s += "." + cls.map((c) => CSS.escape(c)).join(".");
    const p = el.parentElement;
    if (p) {
      const same = [...p.children].filter((c) => c.tagName === el.tagName);
      if (same.length > 1) s += ":nth-of-type(" + (same.indexOf(el) + 1) + ")";
    }
    return s;
  };
  const selector = (el) => {
    const parts = [];
    for (let e = el, d = 0; e && e.nodeType === 1 && e !== document.body && e !== document.documentElement && d < 4; e = e.parentElement, d++) {
      const p = part(e);
      parts.unshift(p);
      if (p[0] === "#") break;
    }
    return parts.join(" > ") || el.tagName.toLowerCase();
  };
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim();
  const name = (el) => squash(el.getAttribute("aria-label") || el.innerText || el.value || el.title || el.getAttribute("alt") || "").slice(0, 60);
  const props = ["outlineStyle", "outlineWidth", "outlineColor", "boxShadow", "borderTopColor", "borderBottomColor", "borderTopWidth",
    "backgroundColor", "color", "textDecorationLine", "opacity", "transform"];
  const look = (el) => {
    const cs = getComputedStyle(el);
    return props.map((p) => cs[p]).join("|");
  };
  // Focus inside an open shadow root shows on the document as its host, so look through to the element itself.
  const activeElement = () => {
    let el = document.activeElement;
    while (el && el.shadowRoot && el.shadowRoot.activeElement) el = el.shadowRoot.activeElement;
    return el;
  };
  const blurActive = () => {
    const el = activeElement();
    if (el && el !== document.body) el.blur();
  };
  const focusables = () =>
    [...document.querySelectorAll('a[href], button, input:not([type=hidden]), select, textarea, summary, [tabindex], [contenteditable=""], [contenteditable="true"], iframe, object, embed, audio[controls], video[controls]')];

  if (op === "init") {
    const list = focusables();
    for (const el of list) S.look.set(el, look(el));
    blurActive();
    // A page opened at a #fragment, or clicked in, leaves the browser's starting point for Tab somewhere down the
    // page. Focusing a throwaway element at the top and removing it, then putting the selection there, moves it back.
    const root = document.body || document.documentElement;
    const mark = document.createElement("span");
    mark.tabIndex = -1;
    root.insertBefore(mark, root.firstChild);
    mark.focus({ preventScroll: true });
    mark.remove();
    blurActive();
    const sel = getSelection();
    if (sel && document.body) sel.collapse(document.body, 0);
    window.scrollTo(0, 0);
    return { focusables: list.length };
  }
  if (op === "blur") {
    blurActive();
    return true;
  }
  if (op === "focus") {
    const el = S.els[arg] && S.els[arg].deref();
    if (el) el.focus({ preventScroll: true });
    return !!el;
  }
  if (op === "skip") {
    const el = activeElement();
    if (!el || el.tagName !== "A") return null;
    const href = el.getAttribute("href") || "";
    const text = squash(el.innerText || el.getAttribute("aria-label"));
    if (href[0] !== "#" || href.length < 2 || !/skip|jump to|go to (the )?(main|content)/i.test(text)) return null;
    let id = href.slice(1);
    try { id = decodeURIComponent(id); } catch (e) {}
    const target = document.getElementById(id) || document.getElementsByName(id)[0];
    return { selector: selector(el), text, href, found: !!target, rendered: !!target && target.getClientRects().length > 0 };
  }
  // "state"
  const el = activeElement();
  if (!el || el === document.body || el === document.documentElement) return { body: true };
  const r = el.getBoundingClientRect();
  const cs = getComputedStyle(el);
  const hidden =
    (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true })) ||
    cs.opacity === "0" || r.width < 1 || r.height < 1;
  const off = r.right <= 0 || r.bottom <= 0 || r.left >= innerWidth || r.top >= innerHeight;
  const known = S.look.has(el);
  const outline = cs.outlineStyle !== "none" && parseFloat(cs.outlineWidth) > 0 && !/rgba\(\d+, \d+, \d+, 0\)/.test(cs.outlineColor);
  let modal = false;
  for (let a = el; a && a.nodeType === 1; a = a.parentElement || a.getRootNode().host) {
    if ((a.tagName === "DIALOG" && a.open) || a.getAttribute("aria-modal") === "true" || a.getAttribute("role") === "dialog" || a.getAttribute("role") === "alertdialog") modal = true;
  }
  return {
    id: idOf(el), selector: selector(el), tag: el.tagName.toLowerCase(), name: name(el),
    box: { x: round(r.left + scrollX), y: round(r.top + scrollY), width: round(r.width), height: round(r.height) },
    hidden: !!hidden, offscreen: off, outline, styleChanged: known && S.look.get(el) !== look(el),
    frame: ["IFRAME", "OBJECT", "EMBED"].includes(el.tagName) || (el.tagName === "VIDEO" || el.tagName === "AUDIO"),
    modal, tabindex: el.getAttribute("tabindex"),
  };
}
