// The accessibility view of the page: which element has focus, and a compact
// outline of landmarks, headings and interactive elements with their roles and
// accessible names. WebDriver BiDi has no accessibility tree to read (its only
// accessibility feature is a locator for finding nodes by role and name), so
// this computes role and name from the DOM and ARIA attributes in the page,
// following the rules in HTML-AAM and Accessible Name and Description
// Computation 1.2 for the common cases. It is an approximation of what the
// browser exposes, not a copy of it.
//
// Runs right after look.js, so the numbered controls already carry
// data-dummyrun-ref and the outline can point at them with the same @numbers.
// Called as a function expression with (maxChars).
(maxChars) => {
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim();
  const clip = (s, n) => (s.length > n ? s.slice(0, n - 1) + "…" : s);

  const hidden = (el) => {
    for (let e = el; e && e.nodeType === 1; e = e.parentElement) {
      if (e.hasAttribute("hidden") || e.getAttribute("aria-hidden") === "true" || e.inert) return true;
      const s = getComputedStyle(e);
      if (s.display === "none" || s.visibility === "hidden") return true;
    }
    return false;
  };
  const rendered = (el) => {
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0 && r.right > 0 && r.left < innerWidth && !hidden(el);
  };

  const byIds = (ids) =>
    (ids || "").split(/\s+/).map((id) => document.getElementById(id)).filter(Boolean);

  // Text a node contributes to a name computed from content.
  const content = (node, seen) => {
    if (node.nodeType === 3) return node.data;
    if (node.nodeType !== 1 || seen.has(node)) return "";
    const el = node;
    const s = getComputedStyle(el);
    if (s.display === "none" || s.visibility === "hidden" || el.getAttribute("aria-hidden") === "true") return "";
    seen.add(el);
    const label = el.getAttribute("aria-label");
    if (label && squash(label)) return " " + squash(label) + " ";
    const tag = el.tagName.toLowerCase();
    if (tag === "img") return squash(el.getAttribute("alt"));
    if (tag === "input" || tag === "textarea" || tag === "select") return "";
    if (tag === "svg") return squash(el.querySelector("title")?.textContent);
    let out = "";
    for (const child of el.childNodes) out += content(child, seen);
    return /^(block|flex|grid|list-item|table)/.test(s.display) ? " " + out + " " : out;
  };

  const implicitRole = (el) => {
    const t = el.tagName.toLowerCase();
    const type = (el.getAttribute("type") || "").toLowerCase();
    const named = !!(el.getAttribute("aria-label") || el.getAttribute("aria-labelledby"));
    switch (t) {
      case "a": case "area": return el.hasAttribute("href") ? "link" : "";
      case "button": case "summary": return "button";
      case "input":
        if (["button", "submit", "reset", "image"].includes(type)) return "button";
        if (type === "checkbox") return "checkbox";
        if (type === "radio") return "radio";
        if (type === "range") return "slider";
        if (type === "number") return "spinbutton";
        if (type === "search") return "searchbox";
        if (type === "hidden") return "";
        return el.hasAttribute("list") ? "combobox" : "textbox";
      case "select": return el.multiple || el.size > 1 ? "listbox" : "combobox";
      case "textarea": return "textbox";
      case "nav": return "navigation";
      case "main": return "main";
      case "aside": return "complementary";
      case "search": return "search";
      case "header": return el.closest("article, aside, main, nav, section") ? "" : "banner";
      case "footer": return el.closest("article, aside, main, nav, section") ? "" : "contentinfo";
      case "form": return named ? "form" : "";
      case "section": return named ? "region" : "";
      case "h1": case "h2": case "h3": case "h4": case "h5": case "h6": return "heading";
      case "img": return "img";
      case "dialog": return "dialog";
      default: return "";
    }
  };
  const roleOf = (el) => {
    const explicit = squash(el.getAttribute("role")).split(" ")[0].toLowerCase();
    if (explicit && explicit !== "none" && explicit !== "presentation") return explicit;
    // isContentEditable is inherited; only the editing host itself is a textbox.
    const host = el.getAttribute("contenteditable");
    if (el.isContentEditable && host !== null && host.toLowerCase() !== "false" && host.toLowerCase() !== "inherit") return "textbox";
    return implicitRole(el);
  };

  const fromContent = new Set([
    "link", "button", "heading", "tab", "menuitem", "menuitemcheckbox", "menuitemradio", "option", "checkbox",
    "radio", "switch", "treeitem", "cell", "gridcell", "columnheader", "rowheader", "tooltip",
  ]);
  const nameOf = (el, role) => {
    const labelledby = byIds(el.getAttribute("aria-labelledby"));
    if (labelledby.length) {
      const n = squash(labelledby.map((e) => content(e, new Set())).join(" "));
      if (n) return n;
    }
    const aria = squash(el.getAttribute("aria-label"));
    if (aria) return aria;
    const t = el.tagName.toLowerCase();
    const type = (el.getAttribute("type") || "").toLowerCase();
    if (t === "input" || t === "select" || t === "textarea") {
      const label = squash([...(el.labels || [])].map((l) => content(l, new Set())).join(" "));
      if (label) return label;
      if (["button", "submit", "reset"].includes(type)) {
        return squash(el.value) || (type === "submit" ? "Submit" : type === "reset" ? "Reset" : "");
      }
      if (type === "image") return squash(el.getAttribute("alt"));
    } else if (t === "img") {
      return squash(el.getAttribute("alt"));
    } else if (t === "fieldset") {
      const legend = el.querySelector(":scope > legend");
      if (legend) return squash(content(legend, new Set()));
    }
    if (fromContent.has(role)) {
      const n = squash(content(el, new Set()));
      if (n) return n;
    }
    const title = squash(el.getAttribute("title"));
    if (title) return title;
    if (t === "input" || t === "textarea") return squash(el.placeholder);
    return "";
  };

  const states = (el) => {
    const out = [];
    if (el.disabled || el.getAttribute("aria-disabled") === "true") out.push("disabled");
    if (el.checked === true || el.getAttribute("aria-checked") === "true") out.push("checked");
    const expanded = el.getAttribute("aria-expanded") ?? (el.tagName === "SUMMARY" ? String(!!el.parentElement?.open) : null);
    if (expanded === "true") out.push("expanded");
    if (expanded === "false") out.push("collapsed");
    if (el.required || el.getAttribute("aria-required") === "true") out.push("required");
    if (el.getAttribute("aria-invalid") === "true") out.push("invalid");
    const current = el.getAttribute("aria-current");
    if (current && current !== "false") out.push("current");
    return out;
  };

  // Focus, looking through open shadow roots and same-origin frames.
  let active = document.activeElement;
  for (let i = 0; i < 5 && active; i++) {
    if (active.shadowRoot?.activeElement) active = active.shadowRoot.activeElement;
    else if (active.tagName === "IFRAME" && active.contentDocument?.activeElement) active = active.contentDocument.activeElement;
    else break;
  }
  const focusable = active && active !== document.body && active !== document.documentElement ? active : null;

  const landmarkRoles = new Set(["banner", "navigation", "main", "complementary", "contentinfo", "search", "form", "region"]);
  const interactive = [
    "a[href]", "button", "input:not([type=hidden])", "select", "textarea", "summary",
    "[role=button]", "[role=link]", "[role=tab]", "[role=option]", "[role=menuitem]",
    "[role=checkbox]", "[role=switch]", "[role=radio]", "[contenteditable=true]", "[tabindex='0']",
  ];
  const all = document.querySelectorAll(
    ["header", "nav", "main", "aside", "footer", "form", "section", "search", "[role]", "h1,h2,h3,h4,h5,h6"].concat(interactive).join(",")
  );

  // First pass: which landmarks exist, so nesting can be counted.
  const landmarks = new Set();
  const roles = new Map();
  for (const el of all) {
    const role = roleOf(el);
    if (!role) continue;
    roles.set(el, role);
    if (landmarkRoles.has(role) && rendered(el)) landmarks.add(el);
  }
  const depthOf = (el) => {
    let d = 0;
    for (let p = el.parentElement; p; p = p.parentElement) if (landmarks.has(p)) d++;
    return Math.min(d, 3);
  };

  const rows = []; // { text, priority, order }
  const interactiveSel = interactive.join(",");
  for (const el of all) {
    const role = roles.get(el);
    if (!role) continue;
    const isFocus = el === focusable;
    const isLandmark = landmarks.has(el);
    const isHeading = role === "heading";
    const isControl = !isLandmark && !isHeading && el.matches(interactiveSel);
    if (!isLandmark && !isHeading && !isControl && !isFocus) continue;
    if (!isFocus && !rendered(el)) continue;
    const name = nameOf(el, role);
    let text = "  ".repeat(depthOf(el)) + role;
    if (isHeading) text += " " + (Number(el.tagName[1]) || el.getAttribute("aria-level") || "");
    text += name ? ' "' + clip(name, isHeading ? 70 : 50) + '"' : isLandmark ? "" : " (no name)";
    const ref = el.getAttribute("data-dummyrun-ref");
    if (ref) text += " @" + ref;
    const st = states(el);
    if (st.length) text += " [" + st.join(", ") + "]";
    if (isFocus) text += "  <- focus";
    rows.push({ text, priority: isFocus || isLandmark ? 0 : isHeading ? 1 : 2, order: rows.length });
  }

  // Keep landmarks and the focused element first, then headings, then
  // controls, until the character budget is spent. The outline stays in page order.
  const reserve = 48;
  let used = 0;
  const keep = new Set();
  for (const priority of [0, 1, 2]) {
    for (const r of rows) {
      if (r.priority !== priority) continue;
      const cost = r.text.length + 1;
      if (used + cost > maxChars - reserve && !(priority === 0 && r.text.includes("<- focus"))) continue;
      keep.add(r.order);
      used += cost;
    }
  }
  const kept = rows.filter((r) => keep.has(r.order)).map((r) => r.text);
  const omitted = rows.length - kept.length;
  if (omitted > 0) kept.push("… " + omitted + " more not shown to stay within the budget");

  let focus = null;
  if (focusable) {
    const role = roles.get(focusable) || roleOf(focusable) || focusable.tagName.toLowerCase();
    focus = {
      role,
      name: clip(nameOf(focusable, role), 80),
      ref: focusable.getAttribute("data-dummyrun-ref") || undefined,
      tag: focusable.tagName.toLowerCase(),
    };
  }
  return { focus, tree: kept.join("\n"), omitted };
}
