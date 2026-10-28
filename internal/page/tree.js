// The one list the persona reads: landmarks, headings H1 to H6 with their levels, and the controls with role,
// accessible name, states and the @number to act on, in page order, plus the element that has keyboard focus.
// Names and exposure come from Chrome's accessibility tree where the engine read it (axtree.go), and otherwise from
// names.js: axe-core's own functions when axe is loaded, and the spec computation otherwise. What assistive
// technology cannot reach is left out.
//
// Runs right after look.js, so the numbered controls already carry data-dummyrun-ref. Called as a function
// expression with (maxChars).
(maxChars) => {
  if (!/^(text\/html|application\/xhtml\+xml)/i.test(document.contentType || "")) return { focus: null, tree: "", omitted: 0 };
/*NAMES*/
  drAxeSetup();
  try {
    // Focus, looking through open shadow roots and same-origin frames.
    let active = document.activeElement;
    for (let i = 0; i < 5 && active; i++) {
      if (active.shadowRoot?.activeElement) active = active.shadowRoot.activeElement;
      else if (active.tagName === "IFRAME" && active.contentDocument?.activeElement) active = active.contentDocument.activeElement;
      else break;
    }
    const focusable = active && active !== document.body && active !== document.documentElement ? active : null;

    const landmarkRoles = new Set(["banner", "navigation", "main", "complementary", "contentinfo", "search", "form", "region"]);
    const chromeRoles = new Set(["banner", "navigation", "contentinfo", "complementary"]);
    const all = document.querySelectorAll(
      ["header", "nav", "main", "aside", "footer", "form", "section", "search", "[role]", "h1,h2,h3,h4,h5,h6"].join(",") + "," + drInteractive
    );

    const landmarks = new Set();
    const roles = new Map();
    for (const el of all) {
      const role = drRole(el);
      if (!role) continue;
      roles.set(el, role);
      if (landmarkRoles.has(role) && drExposed(el)) landmarks.add(el);
    }
    const depthOf = (el) => {
      let d = 0;
      for (let p = el.parentElement; p; p = p.parentElement) if (landmarks.has(p)) d++;
      return Math.min(d, 3);
    };
    // Header, menu, footer and sidebar controls go after the main content's when the budget runs short.
    const inChrome = (el) => {
      for (let p = el.parentElement; p; p = p.parentElement) if (landmarks.has(p) && chromeRoles.has(roles.get(p))) return true;
      return false;
    };

    const rows = []; // { text, priority, order, kind }
    for (const el of all) {
      const role = roles.get(el);
      if (!role) continue;
      const isFocus = el === focusable;
      const isLandmark = landmarks.has(el);
      const isHeading = role === "heading";
      const ref = el.getAttribute("data-dummyrun-ref");
      const isControl = !isLandmark && !isHeading && el.matches(drInteractive);
      if (!isLandmark && !isHeading && !isControl && !isFocus) continue;
      // A numbered control was checked by look.js. Anything else must be exposed to be listed.
      if (!isFocus && !ref && !drExposed(el)) continue;
      const name = drName(el, role);
      let text = "  ".repeat(depthOf(el)) + role;
      const level = isHeading ? Number(el.tagName[1]) || Number(el.getAttribute("aria-level")) || 2 : 0;
      if (isHeading) text += " " + level;
      text += name ? ' "' + drClip(name, 80) + '"' : isLandmark ? "" : " (no name)";
      if (ref) text += " @" + ref;
      const st = drStates(el);
      if (isControl) {
        const r = el.getBoundingClientRect();
        if (r.bottom < 0 || r.top > innerHeight) st.push("off screen");
      }
      if (st.length) text += " [" + st.join(", ") + "]";
      if (isFocus) text += "  <- focus";
      let priority;
      if (isFocus || isLandmark) priority = 0;
      else if (isHeading) priority = level <= 2 ? 1 : level === 3 ? 2 : 3;
      else priority = inChrome(el) ? 4 : 2;
      rows.push({ text, priority, order: rows.length, focus: isFocus, kind: isHeading ? "heading" : isControl ? "control" : "landmark" });
    }

    // Keep landmarks and the focused element, then the top headings, then the main content's controls and H3s,
    // then deeper headings, then header, menu and footer controls, until the budget is spent. Page order is kept.
    const reserve = 80;
    let used = 0;
    const keep = new Set();
    for (const priority of [0, 1, 2, 3, 4]) {
      for (const r of rows) {
        if (r.priority !== priority) continue;
        const cost = r.text.length + 1;
        // Only the focused element may go over the budget. That is a flag on the row, never read from the
        // text, which carries names the page chose.
        if (used + cost > maxChars - reserve && !r.focus) continue;
        keep.add(r.order);
        used += cost;
      }
    }
    const kept = rows.filter((r) => keep.has(r.order)).map((r) => r.text);
    const dropped = rows.filter((r) => !keep.has(r.order));
    if (dropped.length > 0) {
      const c = dropped.filter((r) => r.kind === "control").length;
      const h = dropped.filter((r) => r.kind === "heading").length;
      kept.push("… " + dropped.length + " more not shown to stay within the budget (" + c + " controls, " + h + " headings)");
    }

    let focus = null;
    if (focusable) {
      const role = roles.get(focusable) || drRole(focusable) || focusable.tagName.toLowerCase();
      focus = {
        role,
        name: drClip(drName(focusable, role), 80),
        ref: focusable.getAttribute("data-dummyrun-ref") || undefined,
        tag: focusable.tagName.toLowerCase(),
      };
    }
    return { focus, tree: kept.join("\n"), omitted: dropped.length };
  } finally {
    drAxeTeardown();
  }
}
