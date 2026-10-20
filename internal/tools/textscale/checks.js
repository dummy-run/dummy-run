// Runs in the page. Returns the layout faults found at the current size, as plain data.
(() => {
  const vw = document.documentElement.clientWidth;
  const out = { overflow: [], clipped: [], overlaps: [] };
  const name = (el) => {
    let s = el.tagName.toLowerCase();
    if (el.id) s += "#" + el.id;
    else if (el.classList.length) s += "." + [...el.classList].slice(0, 2).join(".");
    const t = (el.innerText || el.getAttribute("aria-label") || el.value || "").trim().replace(/\s+/g, " ").slice(0, 40);
    return t ? s + ' "' + t + '"' : s;
  };
  const visible = (el) => {
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true, contentVisibilityAuto: true })) return false; // a closed <details> hides its content
    const cs = getComputedStyle(el);
    if (cs.display === "none" || cs.visibility === "hidden" || cs.opacity === "0") return false;
    const r = el.getBoundingClientRect();
    if (r.width < 1 || r.height < 1) return false;
    if (r.right < 0 || r.bottom < 0) return false; // the skip link waits off screen
    return true;
  };
  // An element inside a box that scrolls on its own (a code block, a table) may run past the page edge.
  const insideScroller = (el) => {
    for (let p = el.parentElement; p && p !== document.body; p = p.parentElement) {
      const ox = getComputedStyle(p).overflowX;
      if ((ox === "auto" || ox === "scroll" || ox === "hidden") && p.scrollWidth > p.clientWidth) return true;
    }
    return false;
  };

  // 1. The page itself must not scroll sideways (WCAG 1.4.10 reflow).
  const sw = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
  if (sw > vw) {
    out.pageScrollWidth = sw;
    out.viewportWidth = vw;
    for (const el of document.body.querySelectorAll("*")) {
      if (!visible(el) || insideScroller(el)) continue;
      const r = el.getBoundingClientRect();
      if (r.right > vw + 1 || r.left < -1) out.overflow.push(name(el) + " right=" + Math.round(r.right));
      if (out.overflow.length >= 8) break;
    }
  }

  // 2. Text in a control must fit its box (WCAG 1.4.4 resize text).
  const controls = [...document.querySelectorAll("a[href], button, label, input, select, textarea, summary, [role=button]")].filter(visible);
  for (const el of controls) {
    const cs = getComputedStyle(el);
    if (cs.display === "inline") {
      for (const r of el.getClientRects()) if (r.right > vw + 1 && !insideScroller(el)) { out.clipped.push(name(el) + " runs past the edge"); break; }
      continue;
    }
    const clips = cs.overflowX !== "visible" || cs.overflowY !== "visible";
    const wide = el.scrollWidth > el.clientWidth + 1;
    const tall = el.scrollHeight > el.clientHeight + 1;
    const isField = el.matches("input, textarea, select");
    if (isField && !el.matches("input[type=checkbox], input[type=radio]")) {
      // A field scrolls its own value; what must not happen is the field running past the page edge.
      const r = el.getBoundingClientRect();
      if (r.right > vw + 1) out.clipped.push(name(el) + " field runs past the edge");
      continue;
    }
    if ((clips && (wide || tall)) || (!clips && wide && el.matches("button, .btn, .lib-chip, label"))) {
      out.clipped.push(name(el) + " scrollWidth=" + el.scrollWidth + " clientWidth=" + el.clientWidth + " scrollHeight=" + el.scrollHeight + " clientHeight=" + el.clientHeight);
    }
  }

  // 3. No two controls may sit on top of each other. Wrapped links are compared line by line.
  const boxes = controls.filter((el) => !el.matches("label")).map((el) => ({ el, rects: [...el.getClientRects()].filter((r) => r.width >= 1 && r.height >= 1) }));
  const slop = 2;
  for (let i = 0; i < boxes.length; i++) {
    for (let j = i + 1; j < boxes.length; j++) {
      const a = boxes[i], b = boxes[j];
      if (a.el.contains(b.el) || b.el.contains(a.el)) continue;
      let hit = false;
      for (const ra of a.rects) {
        for (const rb of b.rects) {
          const w = Math.min(ra.right, rb.right) - Math.max(ra.left, rb.left);
          const h = Math.min(ra.bottom, rb.bottom) - Math.max(ra.top, rb.top);
          if (w > slop && h > slop) hit = true;
        }
      }
      if (hit) out.overlaps.push(name(a.el) + " overlaps " + name(b.el));
    }
  }
  return out;
})()
