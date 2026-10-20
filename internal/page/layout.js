// Layout facts about the page as it is now, measured the way a person would see them: sideways
// overflow, interactive elements whose text is cut off, interactive elements that overlap, and
// interactive elements that are too small to hit. No model is involved.
//
// Called as a function expression with (max, minTarget, tolerance): the most items of each kind to
// return, the smallest target side in CSS px, and the px of overlap or clipping to ignore.
(max, minTarget, tol) => {
  const root = document.documentElement;
  const cw = root.clientWidth;
  const round = (n) => Math.round(n * 10) / 10;

  // A short CSS selector for an element: an id if there is one, else the tag with a class and its
  // position among its siblings, joined up the tree until an id or four levels.
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
    for (let e = el, depth = 0; e && e.nodeType === 1 && e !== document.body && e !== root && depth < 4; e = e.parentElement, depth++) {
      const p = part(e);
      parts.unshift(p);
      if (p[0] === "#") break;
    }
    return parts.join(" > ");
  };
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim();
  const label = (el) =>
    squash(el.getAttribute("aria-label") || el.innerText || el.value || el.title || el.getAttribute("alt") || "").slice(0, 60);
  const refOf = (el) => el.getAttribute("data-dummyrun-ref") || undefined;
  const box = (r) => ({ x: round(r.left + scrollX), y: round(r.top + scrollY), width: round(r.width), height: round(r.height) });

  const visible = (el) => {
    const r = el.getBoundingClientRect();
    // 1 px or less is how a page hides text from the eye and keeps it for a screen reader.
    if (r.width <= 1 || r.height <= 1) return false;
    if (r.right <= 0 || r.bottom <= 0) return false;
    const s = getComputedStyle(el);
    if (s.visibility === "hidden" || s.display === "none" || s.opacity === "0") return false;
    for (let a = el; a && a.nodeType === 1; a = a.parentElement) {
      if (a.hasAttribute("hidden") || a.inert) return false;
      const as = getComputedStyle(a);
      if (as.display === "none" || as.visibility === "hidden") return false;
    }
    return true;
  };

  const interactive = [
    "a[href]", "button", "input:not([type=hidden])", "select", "textarea", "summary",
    "[role=button]", "[role=link]", "[role=tab]", "[role=menuitem]", "[role=checkbox]", "[role=switch]",
    "[role=radio]", "[contenteditable=true]", "[tabindex='0']",
  ].join(",");
  const controls = [];
  for (const el of document.querySelectorAll(interactive)) {
    if (controls.length >= 1500) break;
    if (el.disabled || el.getAttribute("aria-disabled") === "true") continue;
    if (!visible(el)) continue;
    controls.push(el);
  }

  // Sideways overflow: the page is wider than the window, so it scrolls sideways.
  let overflow = null;
  const scrollW = Math.max(root.scrollWidth, document.body ? document.body.scrollWidth : 0);
  if (scrollW > cw + tol) {
    const culprits = [];
    const wide = new Set();
    let scanned = 0;
    for (const el of document.body ? document.body.querySelectorAll("*") : []) {
      if (++scanned > 8000) break;
      const r = el.getBoundingClientRect();
      if (r.width === 0 || r.right <= cw + tol) continue;
      const s = getComputedStyle(el);
      if (s.position === "fixed" || s.display === "none" || s.visibility === "hidden") continue;
      // Something that scrolls or clips inside its own box does not widen the page.
      let held = false;
      for (let a = el.parentElement; a && a !== document.body && a !== root; a = a.parentElement) {
        const as = getComputedStyle(a);
        if (as.overflowX !== "visible" && a.getBoundingClientRect().right <= cw + tol) { held = true; break; }
        if (as.position === "fixed") { held = true; break; }
      }
      if (held) continue;
      wide.add(el);
      if (!el.parentElement || !wide.has(el.parentElement)) culprits.push({ el, right: r.right });
    }
    culprits.sort((a, b) => b.right - a.right);
    overflow = {
      scroll_width: scrollW,
      client_width: cw,
      excess: round(scrollW - cw),
      culprits: culprits.slice(0, 5).map((c) => ({ selector: selector(c.el), ref: refOf(c.el), right: round(c.right), label: label(c.el) })),
    };
  }

  // Text cut off inside an interactive element: its own box clips its text, or a parent that clips
  // hides part of the element.
  const clipped = [];
  const clips = (s) => s.overflowX === "hidden" || s.overflowX === "clip" || s.overflowY === "hidden" || s.overflowY === "clip";
  for (const el of controls) {
    if (clipped.length >= max) break;
    if (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)) continue;
    let why = "";
    const parts = [el, ...el.querySelectorAll("*")].slice(0, 25);
    for (const e of parts) {
      const s = getComputedStyle(e);
      if (!clips(s) || e.clientWidth === 0 || !squash(e.textContent)) continue;
      // Text kept for a screen reader, in a 1 px box, is clipped on purpose.
      const er = e.getBoundingClientRect();
      if (er.width <= 1 || er.height <= 1) continue;
      const sideways = e.scrollWidth > e.clientWidth + tol;
      const down = e.scrollHeight > e.clientHeight + tol;
      if ((sideways && (s.overflowX === "hidden" || s.overflowX === "clip")) || (down && (s.overflowY === "hidden" || s.overflowY === "clip"))) {
        why = s.textOverflow === "ellipsis" && sideways
          ? "text is shortened with an ellipsis (" + e.clientWidth + " px wide, text needs " + e.scrollWidth + " px)"
          : "text is cut off inside the element (" + e.clientWidth + " x " + e.clientHeight + " px visible, text needs " + e.scrollWidth + " x " + e.scrollHeight + " px)";
        break;
      }
    }
    if (!why) {
      const r = el.getBoundingClientRect();
      for (let a = el.parentElement; a && a !== document.body && a !== root; a = a.parentElement) {
        const as = getComputedStyle(a);
        if (!clips(as)) continue;
        const ar = a.getBoundingClientRect();
        const cutRight = as.overflowX !== "visible" && r.right > ar.right + tol && r.left < ar.right;
        const cutBottom = as.overflowY !== "visible" && r.bottom > ar.bottom + tol && r.top < ar.bottom;
        if (cutRight || cutBottom) {
          why = "a parent (" + selector(a) + ") hides part of it: " + (cutRight ? round(r.right - ar.right) + " px past its right edge" : round(r.bottom - ar.bottom) + " px past its bottom edge");
          break;
        }
      }
    }
    if (why) clipped.push({ selector: selector(el), ref: refOf(el), label: label(el), detail: why, box: box(el.getBoundingClientRect()) });
  }

  // Overlap: two interactive elements whose boxes share more than tol px in both directions, where
  // neither holds the other.
  const overlaps = [];
  const rects = controls.map((el) => el.getBoundingClientRect());
  const fixedLike = controls.map((el) => { const p = getComputedStyle(el).position; return p === "fixed" || p === "sticky"; });
  let overlapTotal = 0;
  for (let i = 0; i < controls.length; i++) {
    for (let j = i + 1; j < controls.length; j++) {
      const a = rects[i], b = rects[j];
      const w = Math.min(a.right, b.right) - Math.max(a.left, b.left);
      const h = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
      if (w <= tol || h <= tol) continue;
      if (controls[i].contains(controls[j]) || controls[j].contains(controls[i])) continue;
      const cx = Math.max(a.left, b.left) + w / 2, cy = Math.max(a.top, b.top) + h / 2;
      // A bar fixed to the window only covers what is in the window.
      if ((fixedLike[i] || fixedLike[j]) && (cy > innerHeight || cx > innerWidth)) continue;
      // If something else is on top where they meet, neither is seen there.
      if (cx >= 0 && cy >= 0 && cx < innerWidth && cy < innerHeight) {
        const top = document.elementFromPoint(cx, cy);
        if (top && !controls[i].contains(top) && !controls[j].contains(top) && !top.contains(controls[i]) && !top.contains(controls[j])) continue;
      }
      overlapTotal++;
      if (overlaps.length < max) {
        overlaps.push({
          a: { selector: selector(controls[i]), ref: refOf(controls[i]), label: label(controls[i]) },
          b: { selector: selector(controls[j]), ref: refOf(controls[j]), label: label(controls[j]) },
          width: round(w), height: round(h),
          box: box({ left: Math.min(a.left, b.left), top: Math.min(a.top, b.top), width: Math.max(a.right, b.right) - Math.min(a.left, b.left), height: Math.max(a.bottom, b.bottom) - Math.min(a.top, b.top) }),
        });
      }
    }
  }

  // Target size, WCAG 2.5.8: at least minTarget x minTarget CSS px, unless the target is a link in a
  // sentence, or has room around it (a minTarget circle on its centre meets no other target).
  const small = [];
  const isInline = (el) => {
    if (el.tagName !== "A") return false;
    if (getComputedStyle(el).display !== "inline") return false;
    const p = el.parentElement;
    const own = p ? [...p.childNodes].filter((n) => n.nodeType === 3 && squash(n.nodeValue)).length : 0;
    return own > 0;
  };
  const targetBox = (el) => {
    const r = el.getBoundingClientRect();
    // A checkbox or radio is hit through its label too, so the label counts as part of the target.
    if (el.tagName === "INPUT" && /^(checkbox|radio)$/.test(el.type) && el.labels && el.labels.length) {
      let best = r;
      for (const l of el.labels) {
        const lr = l.getBoundingClientRect();
        if (lr.width * lr.height > best.width * best.height) best = lr;
      }
      return best;
    }
    return r;
  };
  const boxes = controls.map(targetBox);
  let smallTotal = 0;
  for (let i = 0; i < controls.length; i++) {
    const r = boxes[i];
    if (r.width >= minTarget - 0.01 && r.height >= minTarget - 0.01) continue;
    if (isInline(controls[i])) continue;
    // Spacing exception.
    const cx = r.left + r.width / 2, cy = r.top + r.height / 2, rad = minTarget / 2;
    let crowded = false;
    for (let j = 0; j < controls.length && !crowded; j++) {
      if (j === i || controls[i].contains(controls[j]) || controls[j].contains(controls[i])) continue;
      const o = boxes[j];
      const dx = Math.max(o.left - cx, 0, cx - o.right), dy = Math.max(o.top - cy, 0, cy - o.bottom);
      if (dx * dx + dy * dy < rad * rad) crowded = true;
    }
    if (!crowded) continue;
    smallTotal++;
    if (small.length < max) small.push({ selector: selector(controls[i]), ref: refOf(controls[i]), label: label(controls[i]), width: round(r.width), height: round(r.height), box: box(r) });
  }

  const font = parseFloat(getComputedStyle(root).fontSize);
  return {
    viewport: [innerWidth, innerHeight],
    root_font_px: font,
    controls: controls.length,
    overflow,
    clipped,
    overlaps,
    overlap_count: overlapTotal,
    small,
    small_count: smallTotal,
  };
}
