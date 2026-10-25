// The page side of the keyboard and mouse comparison. Called as a function expression with (op, arg) in a
// sandbox, so its state lives beside the page and not in it. Every verdict is computed from the page: a task's
// success test must be false when the page has just loaded and true after the action. Ops:
//   "find"     the controls worth trying, up to arg of them, each with how its success is tested
//   "arm"      on a freshly loaded page, find the element of the task in arg, as JSON, and test it: is it there, is the test
//              false, and where would a mouse press it
//   "check"    test the task in arg again, after the action
//   "focused"  does the element named by the selector in arg have keyboard focus
//   "skip"     describe the skip link if it has focus: its target's selector, or null
//   "past"     is focus now inside the element named by the selector in arg, or after it in the page
(op, arg) => {
  const S = (globalThis.__drTasks = globalThis.__drTasks || { base: "" });
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
    for (let e = el, d = 0; e && e.nodeType === 1 && e !== document.body && e !== document.documentElement && d < 6; e = e.parentElement, d++) {
      const p = part(e);
      parts.unshift(p);
      if (p[0] === "#") break;
    }
    return parts.join(" > ") || el.tagName.toLowerCase();
  };
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim();
  const label = (el) => squash(el.getAttribute("aria-label") || el.innerText || el.title || "").slice(0, 60);
  const shown = (el) => !!el && el.isConnected && (!el.checkVisibility || el.checkVisibility({ visibilityProperty: true, opacityProperty: true })) &&
    el.getBoundingClientRect().width > 1 && el.getBoundingClientRect().height > 1;
  const activeElement = () => {
    let el = document.activeElement;
    while (el && el.shadowRoot && el.shadowRoot.activeElement) el = el.shadowRoot.activeElement;
    return el;
  };
  const interactive = "a[href], button, input, select, textarea, summary, [role=button], [role=link], [role=tab], [role=menuitem], [role=checkbox], [role=switch], [role=radio], [tabindex], [contenteditable=true]";
  // What a person would notice changing: the address, the words on the page and how many elements are shown.
  const signature = () => {
    let n = 0, seen = 0;
    for (const el of document.body ? document.body.querySelectorAll("*") : []) {
      if (++seen > 4000) break;
      if (el.checkVisibility && el.checkVisibility({ visibilityProperty: true })) n++;
    }
    return location.href + "|" + (document.body ? document.body.innerText.length : 0) + "|" + n;
  };
  const one = (sel) => {
    try {
      const list = document.querySelectorAll(sel);
      return list.length === 1 ? list[0] : null;
    } catch (e) {
      return null;
    }
  };
  // The success test of a task, true once the action has had its effect.
  const passed = (t, el) => {
    switch (t.kind) {
      case "expanded": return el.getAttribute("aria-expanded") === "true";
      case "controls": return shown(document.getElementById(t.target));
      case "details": return !!(el.parentElement && el.parentElement.open);
      case "selected": return el.getAttribute("aria-selected") === "true";
      case "checked": return el.getAttribute("aria-checked") === "true";
      case "change": return signature() !== S.base;
    }
    return false;
  };
  // A control that would leave the page or send a form is never tried.
  const leaves = (el) => {
    const a = el.closest("a[href]");
    if (a && !/^#/.test(a.getAttribute("href") || "") && !/^javascript:/i.test(a.getAttribute("href") || "")) return true;
    const b = el.closest("button, input");
    if (b && b.form && (b.type === "submit" || b.type === "image")) return true;
    return false;
  };

  if (op === "find") {
    const out = [], used = new Set();
    const add = (el, kind, target) => {
      if (out.length >= arg || used.has(el) || !shown(el) || el.disabled || el.getAttribute("aria-disabled") === "true" || leaves(el)) return;
      const sel = selector(el);
      if (one(sel) !== el) return;
      used.add(el);
      out.push({ selector: sel, kind, target: target || "", label: label(el) });
    };
    for (const el of document.querySelectorAll("[aria-expanded=false]")) add(el, "expanded");
    for (const el of document.querySelectorAll("[aria-controls]")) {
      const id = (el.getAttribute("aria-controls") || "").split(/\s+/)[0];
      const target = id && document.getElementById(id);
      if (target && !shown(target)) add(el, "controls", id);
    }
    for (const el of document.querySelectorAll("details:not([open]) > summary")) add(el, "details");
    for (const el of document.querySelectorAll("[role=tab][aria-selected=false]")) add(el, "selected");
    for (const el of document.querySelectorAll("[role=checkbox][aria-checked=false]:not(input), [role=switch][aria-checked=false]:not(input)")) add(el, "checked");
    // Something that looks clickable, where the pointer turns into a hand, but is not a control. Its parent's
    // pointer is not a hand, so this is where the look starts, not an inherited style.
    let seen = 0;
    for (const el of document.body ? document.body.querySelectorAll("*") : []) {
      if (out.length >= arg || ++seen > 4000) break;
      if (el.closest(interactive) || getComputedStyle(el).cursor !== "pointer") continue;
      const p = el.parentElement;
      if (p && getComputedStyle(p).cursor === "pointer") continue;
      if (!squash(el.innerText) && !el.querySelector("img, svg")) continue;
      add(el, "change");
    }
    return out;
  }
  if (op === "arm" || op === "check") {
    arg = JSON.parse(arg); // the task, as JSON
    const el = one(arg.selector);
    if (!el) return { found: false };
    if (op === "arm") S.base = signature();
    const pass = passed(arg, el);
    el.scrollIntoView({ block: "center", inline: "center" });
    const r = el.getBoundingClientRect();
    const x = r.left + r.width / 2, y = r.top + r.height / 2;
    const top = document.elementFromPoint(x, y);
    // The mouse presses what is on top. If that is part of the element, a click there reaches it.
    const hittable = !!top && (top === el || el.contains(top));
    return { found: true, pass, x: Math.round(x), y: Math.round(y), hittable };
  }
  if (op === "focused") {
    const el = one(arg);
    const a = activeElement();
    return !!el && !!a && (a === el || el.contains(a));
  }
  if (op === "skip") {
    const a = activeElement();
    if (!a || a.tagName !== "A") return null;
    const href = a.getAttribute("href") || "";
    if (href[0] !== "#" || href.length < 2 || !/skip|jump to|go to (the )?(main|content)/i.test(squash(a.innerText || a.getAttribute("aria-label")))) return null;
    let t = null;
    try {
      t = document.getElementById(decodeURIComponent(href.slice(1)));
    } catch (e) {
      t = null;
    }
    if (!t || !shown(t)) return null; // a missing or hidden target is the Tab walk's finding
    return { link: selector(a), target: selector(t), label: label(a) };
  }
  if (op === "past") {
    const t = one(arg);
    const a = activeElement();
    if (!t || !a || a === document.body) return false;
    return t === a || t.contains(a) || !!(t.compareDocumentPosition(a) & Node.DOCUMENT_POSITION_FOLLOWING);
  }
  return null;
}
