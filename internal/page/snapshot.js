// An outline of the page's accessibility tree in the style of Playwright's ARIA snapshots: landmarks, headings and
// controls, in page order, each as a YAML list item with its role, its accessible name and its states. A landmark's
// contents sit under it, indented. Text, the persona's @numbers and focus are left out, so the outline changes when
// the page's structure or controls change and not when its words do. Called as a function expression with
// (maxLines).
(maxLines) => {
  if (!/^(text\/html|application\/xhtml\+xml)/i.test(document.contentType || "")) return { lines: [], omitted: 0 };
/*NAMES*/
  drAxeSetup();
  try {
    const landmarkRoles = new Set(["banner", "navigation", "main", "complementary", "contentinfo", "search", "form", "region"]);
    const all = document.querySelectorAll(
      ["header", "nav", "main", "aside", "footer", "form", "section", "search", "[role]", "h1,h2,h3,h4,h5,h6"].join(",") + "," + drInteractive
    );
    const landmarks = new Set();
    const roles = new Map();
    for (const el of all) {
      const role = drRole(el);
      if (!role || !drExposed(el)) continue;
      roles.set(el, role);
      if (landmarkRoles.has(role)) landmarks.add(el);
    }
    const depthOf = (el) => {
      let d = 0;
      for (let p = el.parentElement; p; p = p.parentElement) if (landmarks.has(p)) d++;
      return Math.min(d, 6);
    };
    // A name goes in double quotes, so quotes and backslashes in it are escaped, as YAML does.
    const quoted = (s) => '"' + drClip(s, 100).replace(/\\/g, "\\\\").replace(/"/g, '\\"') + '"';
    const lines = [];
    let omitted = 0;
    for (const el of all) {
      const role = roles.get(el);
      if (!role) continue;
      const isLandmark = landmarks.has(el);
      const isHeading = role === "heading";
      if (!isLandmark && !isHeading && !el.matches(drInteractive)) continue;
      if (lines.length >= maxLines) {
        omitted++;
        continue;
      }
      const name = drName(el, role);
      let line = "  ".repeat(depthOf(el)) + "- " + role + (name ? " " + quoted(name) : "");
      const attrs = [];
      if (isHeading) attrs.push("level=" + (Number(el.tagName[1]) || Number(el.getAttribute("aria-level")) || 2));
      attrs.push(...drStates(el));
      if (attrs.length) line += " [" + attrs.join("] [") + "]";
      if (isLandmark) line += ":";
      lines.push(line);
    }
    return { lines, omitted };
  } finally {
    drAxeTeardown();
  }
}
