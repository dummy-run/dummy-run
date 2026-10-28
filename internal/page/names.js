  // names.js: role, accessible name, states and exposure, shared by look.js, tree.js and the a11y tool. The engine
  // splices this text into each of them where they say NAMES, so the control list, the outline and the accessibility
  // check can never name the same element two ways.
  //
  // Names and exposure come from axe-core's own functions (axe.commons.text.accessibleText and
  // axe.commons.dom.isVisibleToScreenReaders) when the engine has loaded its axe into the page, and from the spec
  // computation below otherwise. axe misses a <label for> on a button with a role, so an empty axe name falls back to the spec one.
  //
  // In Chrome the engine reads the browser's own accessibility tree first and marks each element it names with
  // data-dummyrun-ax, keyed into the table it leaves at __drAX in this sandbox (axtree.go). A marked element takes
  // Chrome's role, name, states and exposure; anything unmarked is worked out here.
  const drSquash = (s) => (s || "").replace(/\s+/g, " ").trim();
  // Cut a name at a word boundary, never inside a word.
  const drClip = (s, n) => {
    if (s.length <= n) return s;
    let cut = s.slice(0, n);
    const i = cut.lastIndexOf(" ");
    if (i > n / 2) cut = cut.slice(0, i);
    return cut.replace(/[\s.,;:–-]+$/, "") + "…";
  };
  const drAX = (() => {
    try {
      const t = globalThis.__drAX;
      return t && typeof t === "object" ? t : null;
    } catch (e) {
      return null;
    }
  })();
  // Chrome's entry for an element, or null.
  const drAXOf = (el) => {
    if (!drAX || !el.getAttribute) return null;
    const id = el.getAttribute("data-dummyrun-ax");
    return id && Object.prototype.hasOwnProperty.call(drAX, id) ? drAX[id] : null;
  };
  const drAxe = (() => {
    try {
      // The engine's own axe, loaded under a name the page does not know. A page's window.axe is never used.
      const a = window.__DR_AXE__;
      return a && typeof a.setup === "function" && typeof a.teardown === "function" && a.commons?.text?.accessibleText &&
        a.commons?.dom?.isVisibleToScreenReaders ? a : null;
    } catch (e) {
      return null;
    }
  })();
  let drAxeReady = false;
  const drAxeSetup = () => {
    if (!drAxe || drAxeReady) return drAxeReady;
    try {
      drAxe.setup(document);
      drAxeReady = true;
    } catch (e) {
      try {
        drAxe.teardown();
        drAxe.setup(document);
        drAxeReady = true;
      } catch (e2) {
        drAxeReady = false;
      }
    }
    return drAxeReady;
  };
  const drAxeTeardown = () => {
    if (!drAxeReady) return;
    try {
      drAxe.teardown();
    } catch (e) {
      // nothing to undo
    }
    drAxeReady = false;
  };
  // Inside a closed <details>, everything but its own summary is not rendered, even where it still has a box.
  const drInClosedDetails = (el) => {
    for (let d = el.closest("details:not([open])"); d; d = d.parentElement?.closest("details:not([open])")) {
      const summary = d.querySelector(":scope > summary");
      if (!summary || !summary.contains(el)) return true;
    }
    return false;
  };
  // On screen in the layout: a box, on the canvas sideways (a drawer parked off the edge counts as hidden), not
  // display none, visibility hidden or skipped by content-visibility, and not inside a closed <details>.
  const drRendered = (el) => {
    const r = el.getBoundingClientRect();
    if (!(r.width > 0 && r.height > 0 && r.right > 0 && r.left < innerWidth)) return false;
    const s = getComputedStyle(el);
    if (s.display === "none" || s.visibility === "hidden") return false;
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true, contentVisibilityAuto: false })) return false;
    return !drInClosedDetails(el);
  };
  const drHiddenFromAT = (el) => {
    for (let e = el; e && e.nodeType === 1; e = e.parentElement) {
      // The hidden attribute is left to the rendering check: author CSS can show an element that has it.
      if (e.getAttribute("aria-hidden") === "true" || e.inert) return true;
    }
    return false;
  };
  // Exposed: what a screen reader user can reach. Only exposed controls are listed and numbered.
  const drExposed = (el) => {
    if (!drRendered(el)) return false;
    const ax = drAXOf(el);
    if (ax) return !ax.i;
    if (drHiddenFromAT(el)) return false;
    if (drAxeReady) {
      try {
        return !!drAxe.commons.dom.isVisibleToScreenReaders(el);
      } catch (e) {
        return true;
      }
    }
    return true;
  };
  const drByIds = (ids) => (ids || "").split(/\s+/).map((id) => document.getElementById(id)).filter(Boolean);
  // Text a node gives a name computed from content. Unrendered text gives nothing, except under aria-labelledby,
  // which may point at hidden text on purpose.
  const drContent = (node, seen, hiddenOK) => {
    if (node.nodeType === 3) return node.data;
    if (node.nodeType !== 1 || seen.has(node)) return "";
    const el = node;
    const s = getComputedStyle(el);
    if (!hiddenOK && (s.display === "none" || s.visibility === "hidden" || el.getAttribute("aria-hidden") === "true")) return "";
    if (!hiddenOK && el.checkVisibility && !el.checkVisibility({ visibilityProperty: true, contentVisibilityAuto: false })) return "";
    seen.add(el);
    const aria = drSquash(el.getAttribute("aria-label"));
    if (aria) return " " + aria + " ";
    const tag = el.tagName.toLowerCase();
    if (tag === "img" || tag === "area") return " " + drSquash(el.getAttribute("alt")) + " ";
    if (tag === "input" && (el.getAttribute("type") || "").toLowerCase() === "image") return " " + drSquash(el.getAttribute("alt")) + " ";
    if (tag === "input" || tag === "textarea" || tag === "select") return "";
    if (tag === "svg") return " " + drSquash(el.querySelector("title")?.textContent) + " ";
    let out = "";
    for (const child of el.childNodes) out += drContent(child, seen, hiddenOK);
    return /^(block|flex|grid|list-item|table)/.test(s.display) ? " " + out + " " : out;
  };
  const drImplicitRole = (el) => {
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
  const drRole = (el) => {
    const ax = drAXOf(el);
    if (ax && typeof ax.r === "string") return ax.r;
    const explicit = drSquash(el.getAttribute("role")).split(" ")[0].toLowerCase();
    if (explicit && explicit !== "none" && explicit !== "presentation") return explicit;
    // isContentEditable is inherited; only the editing host itself is a textbox.
    const host = el.getAttribute("contenteditable");
    if (el.isContentEditable && host !== null && host.toLowerCase() !== "false" && host.toLowerCase() !== "inherit") return "textbox";
    return drImplicitRole(el);
  };
  const drFromContent = new Set([
    "link", "button", "heading", "tab", "menuitem", "menuitemcheckbox", "menuitemradio", "option", "checkbox",
    "radio", "switch", "treeitem", "cell", "gridcell", "columnheader", "rowheader", "tooltip",
  ]);
  // The spec computation (Accessible Name and Description Computation 1.2 and HTML-AAM, for the common cases).
  const drSpecName = (el, role) => {
    const labelledby = drByIds(el.getAttribute("aria-labelledby"));
    if (labelledby.length) {
      const n = drSquash(labelledby.map((e) => drContent(e, new Set(), true)).join(" "));
      if (n) return n;
    }
    const aria = drSquash(el.getAttribute("aria-label"));
    if (aria) return aria;
    const t = el.tagName.toLowerCase();
    const type = (el.getAttribute("type") || "").toLowerCase();
    // Every labelable element, buttons included, takes its <label> elements.
    const label = drSquash([...(el.labels || [])].map((l) => drContent(l, new Set(), false)).join(" "));
    if (label) return label;
    if (t === "input") {
      if (["button", "submit", "reset"].includes(type)) return drSquash(el.value) || (type === "submit" ? "Submit" : type === "reset" ? "Reset" : "");
      if (type === "image") {
        const alt = drSquash(el.getAttribute("alt"));
        if (alt) return alt;
      }
    } else if (t === "img") {
      return drSquash(el.getAttribute("alt"));
    } else if (t === "fieldset") {
      const legend = el.querySelector(":scope > legend");
      if (legend) return drSquash(drContent(legend, new Set(), false));
    }
    if (drFromContent.has(role)) {
      // From the children, so the element's own aria-hidden does not empty its name.
      const n = drSquash([...el.childNodes].map((c) => drContent(c, new Set(), false)).join(""));
      if (n) return n;
    }
    const title = drSquash(el.getAttribute("title"));
    if (title) return title;
    if (t === "input" || t === "textarea") return drSquash(el.placeholder);
    return "";
  };
  const drName = (el, role) => {
    const ax = drAXOf(el);
    if (ax) return drSquash(ax.n);
    let n = "";
    if (drAxeReady) {
      try {
        n = drSquash(drAxe.commons.text.accessibleText(el));
      } catch (e) {
        n = "";
      }
    }
    return n || drSpecName(el, role);
  };
  const drStates = (el) => {
    const ax = drAXOf(el);
    if (ax) {
      const out = [...(ax.s || [])];
      const current = el.getAttribute("aria-current");
      if (current && current !== "false") out.push("current");
      return out;
    }
    const out = [];
    if (el.matches(":disabled") || el.getAttribute("aria-disabled") === "true") out.push("disabled");
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
  const drInteractive = [
    "a[href]", "button", "input:not([type=hidden])", "select", "textarea", "summary",
    "[role=button]", "[role=link]", "[role=tab]", "[role=option]", "[role=menuitem]",
    "[role=checkbox]", "[role=switch]", "[role=radio]", "[role=combobox]", "[contenteditable=true]", "[tabindex='0']",
  ].join(",");
  // NAMES end
