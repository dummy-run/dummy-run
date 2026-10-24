// Layout facts about the page as it is now, measured from the rectangles the browser lays out and from what is
// painted on top at a point, the way a person would see them. No model is involved. The approach follows the
// geometry scorers of LayoutLens (MIT), written afresh for this engine.
//
//   reflow    the page scrolls sideways, named by the outermost elements that reach past the window. Content
//             WCAG 1.4.10 lets keep a two-dimensional layout (data tables, images, maps, video) does not count.
//   clipped   text that is partly cut off by a box that hides its overflow. Text wholly outside the box, such as
//             the other slides of a carousel, is hidden rather than cut off and is not counted.
//   overlaps  a control whose text, or whole box when it has none, is under another control where they meet,
//             found by asking the browser what is on top at points in the shared area
//   small     targets under minTarget CSS px, with the WCAG 2.5.8 exceptions that can be measured
//   obscured  with focus on, a control that gets focus but is entirely hidden by something opaque (WCAG 2.4.11)
//
// Called as a function expression with (max, minTarget, tolerance, focus): the most items of each kind to return,
// the smallest target side in CSS px, the px of overlap or clipping to ignore, and whether to move focus to each
// control. Focus is moved only when asked, because a page can react to it.
(max, minTarget, tol, focus) => {
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
    return parts.join(" > ") || el.tagName.toLowerCase();
  };
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim();
  const label = (el) =>
    squash(el.getAttribute("aria-label") || el.innerText || el.value || el.title || el.getAttribute("alt") || "").slice(0, 60);
  // Only the number the engine wrote counts. A page could change the attribute afterwards.
  const refOf = (el) => (/^[0-9]{1,4}$/.test(el.getAttribute("data-dummyrun-ref") || "") ? el.getAttribute("data-dummyrun-ref") : undefined);
  const named = (el) => ({ selector: selector(el), ref: refOf(el), label: label(el) });
  // Rectangles are measured before anything scrolls, so the page offset at the start places them in the page.
  const sx = scrollX, sy = scrollY;
  const box = (r) => ({ x: round(r.left + sx), y: round(r.top + sy), width: round(r.width), height: round(r.height) });
  const boxNow = (r) => ({ x: round(r.left + scrollX), y: round(r.top + scrollY), width: round(r.width), height: round(r.height) });

  const visible = (el) => {
    // Content of a closed <details> keeps a box in some browsers but is not shown.
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true, contentVisibilityAuto: true })) return false;
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

  // What is painted on top at a point, looking inside open shadow roots.
  const hit = (x, y) => {
    let h = document.elementFromPoint(x, y);
    while (h && h.shadowRoot) {
      const d = h.shadowRoot.elementFromPoint(x, y);
      if (!d || d === h) break;
      h = d;
    }
    return h;
  };
  const up = (n) => n.parentElement || (n.getRootNode && n.getRootNode().host) || null;
  const within = (outer, n) => {
    for (let e = n; e; e = up(e)) if (e === outer) return true;
    return false;
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
  const isControl = new Set(controls);
  const controlOf = (n) => {
    for (let e = n; e; e = up(e)) if (isControl.has(e)) return e;
    return null;
  };

  // Points in a rectangle, an n by n grid, kept inside the window.
  const grid = (r, n) => {
    const pts = [];
    for (let yi = 0; yi < n; yi++) {
      for (let xi = 0; xi < n; xi++) {
        const x = r.left + r.width * (xi + 0.5) / n, y = r.top + r.height * (yi + 0.5) / n;
        if (x >= 0 && y >= 0 && x < innerWidth && y < innerHeight) pts.push([x, y]);
      }
    }
    return pts;
  };
  // Brings a rectangle into the window, so the browser can say what is on top there, and returns it moved.
  const reveal = (r) => {
    if (r.top >= 0 && r.bottom <= innerHeight && r.left >= 0 && r.right <= innerWidth) return r;
    const dy = r.top + r.height / 2 - innerHeight / 2, dx = r.left + r.width / 2 - innerWidth / 2;
    const bx = scrollX, by = scrollY;
    scrollTo(bx + (r.left < 0 || r.right > innerWidth ? dx : 0), by + dy);
    const mx = scrollX - bx, my = scrollY - by;
    return { left: r.left - mx, right: r.right - mx, top: r.top - my, bottom: r.bottom - my, width: r.width, height: r.height };
  };

  // Reflow, WCAG 1.4.10: the page is wider than the window, so it scrolls sideways.
  const twoD = "table, img, picture, video, canvas, svg, iframe, object, embed, map, math, [role=img], [role=grid], [role=application], [role=toolbar]";
  let overflow = null, reflowExempt = 0;
  const scrollW = Math.max(root.scrollWidth, document.body ? document.body.scrollWidth : 0);
  if (scrollW > cw + tol) {
    const culprits = [];
    let exempt = 0, scanned = 0;
    const out = (r) => r.right > cw + tol || r.left < -tol;
    for (const el of document.body ? document.body.querySelectorAll("*") : []) {
      if (++scanned > 8000) break;
      const r = el.getBoundingClientRect();
      if (r.width === 0 || !out(r)) continue;
      const s = getComputedStyle(el);
      if (s.position === "fixed" || s.display === "none" || s.visibility === "hidden") continue;
      // Only the outermost element that reaches past the window, so a wide block is named once and not with
      // every child. body and html stretch to fit, so they never count as that outer element.
      const p = el.parentElement;
      if (p && p !== document.body && p !== root && out(p.getBoundingClientRect())) continue;
      // Something that scrolls or clips inside its own box, or sits in a fixed bar, does not widen the page.
      let held = false;
      for (let a = el.parentElement; a && a !== document.body && a !== root; a = a.parentElement) {
        const as = getComputedStyle(a);
        if (as.overflowX !== "visible" && !out(a.getBoundingClientRect())) { held = true; break; }
        if (as.position === "fixed") { held = true; break; }
      }
      if (held) continue;
      if (el.closest(twoD)) { exempt++; continue; }
      culprits.push({ el, right: r.right });
    }
    reflowExempt = exempt;
    if (culprits.length || !exempt) {
      culprits.sort((a, b) => b.right - a.right);
      overflow = {
        scroll_width: scrollW,
        client_width: cw,
        excess: round(scrollW - cw),
        culprits: culprits.slice(0, 5).map((c) => ({ ...named(c.el), right: round(c.right) })),
      };
    }
  }

  // Clipped text, WCAG 1.4.4 and 1.4.10: a line of text reaches past the edge of a box that hides its overflow,
  // while part of it is still inside, so the reader sees only part of it.
  const clipped = [];
  let clippedTotal = 0;
  const clipCache = new Map();
  // The boxes above a node that cut off what reaches past them, nearest first. A box that can be scrolled lets
  // the reader reach the rest, so it does not cut anything off, and on that axis nothing further out does either:
  // the text sits in the scrolling box, not in the boxes around it.
  const clippers = (el) => {
    if (clipCache.has(el)) return clipCache.get(el);
    const list = [];
    const scrolls = (v) => v === "auto" || v === "scroll";
    let scrollX = false, scrollY = false;
    for (let a = el; a && a !== root; a = a.parentElement) {
      const s = getComputedStyle(a);
      const x = !scrollX && (s.overflowX === "hidden" || s.overflowX === "clip");
      const y = !scrollY && (s.overflowY === "hidden" || s.overflowY === "clip");
      if (x || y) list.push({ a, s, x, y });
      scrollX = scrollX || scrolls(s.overflowX);
      scrollY = scrollY || scrolls(s.overflowY);
      if (scrollX && scrollY) break;
    }
    clipCache.set(el, list);
    return list;
  };
  const textOwners = new Set();
  const walker = document.body ? document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT) : null;
  let texts = 0;
  for (let t = walker && walker.nextNode(); t && texts < 4000; t = walker.nextNode()) {
    const el = t.parentElement;
    if (!el || !squash(t.nodeValue) || /^(SCRIPT|STYLE|NOSCRIPT|TEMPLATE|OPTION)$/.test(el.tagName)) continue;
    texts++;
    if (textOwners.has(el) || !visible(el)) continue;
    const list = clippers(el);
    if (!list.length) continue;
    const range = document.createRange();
    range.selectNodeContents(t);
    const rects = [...range.getClientRects()].filter((r) => r.width > 0 && r.height > 0);
    let why = "";
    for (const { a, s, x, y } of list) {
      const ar = a.getBoundingClientRect();
      // A 1 px box, or one clipped to nothing, hides text from the eye on purpose.
      if (ar.width <= 1 || ar.height <= 1 || (s.clip && s.clip !== "auto") || (s.clipPath && s.clipPath !== "none")) { why = "skip"; break; }
      // Text with no line inside this box is hidden by it, not cut off: the other slides of a carousel, say.
      // Once a line is inside, every line that reaches past a clipped edge is text the reader loses.
      const inside = rects.some((r) => r.right > ar.left + tol && r.left < ar.right - tol && r.bottom > ar.top + tol && r.top < ar.bottom - tol);
      if (!inside) continue;
      let past = 0;
      for (const r of rects) {
        if (x) past = Math.max(past, r.right - ar.right, ar.left - r.left);
        if (y) past = Math.max(past, r.bottom - ar.bottom, ar.top - r.top);
      }
      if (past <= tol) continue;
      // A line shortened with an ellipsis, or a teaser clamped to some lines, is cut on purpose. It counts only
      // in a control, where the cut-off words are the control's name.
      const shortened = s.textOverflow === "ellipsis" || (s.webkitLineClamp && s.webkitLineClamp !== "none");
      if (shortened && !controlOf(el)) { why = "skip"; break; }
      why = (shortened ? "text is shortened" : "text is cut off") + " by " + (a === el ? "its own box" : selector(a)) +
        ", which hides " + round(past) + " px of it";
      break;
    }
    if (!why || why === "skip") continue;
    textOwners.add(el);
    clippedTotal++;
    const owner = controlOf(el) || el;
    if (clipped.length < max) clipped.push({ ...named(owner), control: !!controlOf(el), detail: why, box: box(owner.getBoundingClientRect()) });
  }

  // Overlap: where two controls meet, ask the browser what is on top. It counts when the text of one control,
  // or its whole box if it has no text, is under the other, so the reader can't see or reach it.
  const overlaps = [];
  let overlapTotal = 0;
  const rects = controls.map((el) => el.getBoundingClientRect());
  const lines = controls.map((el, i) => { const l = [...el.getClientRects()].filter((r) => r.width > 0 && r.height > 0); return l.length ? l : [rects[i]]; });
  const textRects = (el) => {
    const out = [];
    const w = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    for (let t = w.nextNode(); t && out.length < 6; t = w.nextNode()) {
      if (!squash(t.nodeValue)) continue;
      const range = document.createRange();
      range.selectNodeContents(t);
      for (const r of range.getClientRects()) if (r.width > 1 && r.height > 1) out.push(r);
    }
    return out;
  };
  // Of the points on what shows a's content, how many have the other control on top.
  const covered = (a, b) => {
    const tr = textRects(a);
    const areas = tr.length ? tr : [a.getBoundingClientRect()];
    let n = 0, of = 0;
    for (const r0 of areas) {
      const r = reveal(r0);
      for (const [x, y] of grid(r, 3)) {
        of++;
        const h = hit(x, y);
        if (h && within(b, h) && !within(a, h)) n++;
      }
    }
    return { n, of, text: tr.length > 0 };
  };
  for (let i = 0; i < controls.length; i++) {
    for (let j = i + 1; j < controls.length; j++) {
      const a = rects[i], b = rects[j];
      if (a.right <= b.left + tol || b.right <= a.left + tol || a.bottom <= b.top + tol || b.bottom <= a.top + tol) continue;
      // A link that wraps has one box for each line. Compare line by line, so the empty corner of the
      // bounding box is not counted as shared.
      let meet = false;
      for (const la of lines[i]) {
        for (const lb of lines[j]) {
          if (Math.min(la.right, lb.right) - Math.max(la.left, lb.left) > tol && Math.min(la.bottom, lb.bottom) - Math.max(la.top, lb.top) > tol) meet = true;
        }
      }
      if (!meet || within(controls[i], controls[j]) || within(controls[j], controls[i])) continue;
      let under = controls[i], over = controls[j], c = covered(under, over);
      const d = covered(over, under);
      if (d.n > c.n) { under = controls[j]; over = controls[i]; c = d; }
      // Text counts from two points, a box with no text only when most of it is covered.
      if (c.text ? c.n < 2 : c.n * 2 <= c.of) continue;
      overlapTotal++;
      if (overlaps.length < max) {
        overlaps.push({
          a: named(under), b: named(over),
          covered: c.n, sampled: c.of, text: c.text,
          box: box({ left: Math.min(a.left, b.left), top: Math.min(a.top, b.top), width: Math.max(a.right, b.right) - Math.min(a.left, b.left), height: Math.max(a.bottom, b.bottom) - Math.min(a.top, b.top) }),
        });
      }
    }
  }
  scrollTo(sx, sy);

  // Target size, WCAG 2.5.8: at least minTarget x minTarget CSS px, unless the target is a link in a sentence,
  // has room around it (a minTarget circle on its centre meets no other target), or is a control the page left at
  // the browser's own size. The equivalent and essential exceptions need a person and are not guessed.
  const small = [];
  const isInline = (el) => {
    if (!getComputedStyle(el).display.startsWith("inline") || el.tagName !== "A") return false;
    const p = el.parentElement;
    return !!p && [...p.childNodes].some((n) => n.nodeType === 3 && squash(n.nodeValue));
  };
  const sizeProps = /^(width|height|min-width|min-height|max-width|max-height|inline-size|block-size|min-inline-size|min-block-size|padding|padding-(top|right|bottom|left)|border|border-(top|right|bottom|left)(-width)?|border-width|font|font-size|zoom|transform|appearance|box-sizing)$/;
  let sheetsReadable = true;
  const rules = [];
  try {
    const collect = (list) => {
      for (const r of list) {
        if (r.selectorText && r.style) rules.push(r);
        if (r.cssRules) collect(r.cssRules);
      }
    };
    for (const sh of document.styleSheets) collect(sh.cssRules);
  } catch (e) {
    sheetsReadable = false; // a stylesheet from another origin can't be read, so the exception can't be proven
  }
  const sizes = (style) => [...style].some((p) => sizeProps.test(p));
  const browserSized = (el) => {
    if (!sheetsReadable || !/^(BUTTON|INPUT|SELECT|TEXTAREA)$/.test(el.tagName) || sizes(el.style)) return false;
    for (const r of rules) {
      try {
        if (el.matches(r.selectorText) && sizes(r.style)) return false;
      } catch (e) {
        // a selector this browser does not understand cannot style the control
      }
    }
    return true;
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
      if (j === i || within(controls[i], controls[j]) || within(controls[j], controls[i])) continue;
      const o = boxes[j];
      const dx = Math.max(o.left - cx, 0, cx - o.right), dy = Math.max(o.top - cy, 0, cy - o.bottom);
      if (dx * dx + dy * dy < rad * rad) crowded = true;
      // Another undersized target counts when the two circles meet, so their centres are under a diameter apart.
      if (!crowded && (o.width < minTarget - 0.01 || o.height < minTarget - 0.01)) {
        const ox = o.left + o.width / 2 - cx, oy = o.top + o.height / 2 - cy;
        if (ox * ox + oy * oy < minTarget * minTarget) crowded = true;
      }
    }
    if (!crowded || browserSized(controls[i])) continue;
    smallTotal++;
    if (small.length < max) small.push({ ...named(controls[i]), width: round(r.width), height: round(r.height), box: box(r) });
  }

  // Focus not obscured, WCAG 2.4.11: give each control focus, let the browser scroll it into view as it does for
  // a keyboard user, and look at what is on top across it. It fails only when every point is under something
  // opaque whose box holds the whole visible part of the control. Partly hidden is allowed at level AA.
  const obscured = [];
  let obscuredTotal = 0, focused = 0;
  if (focus) {
    const before = document.activeElement;
    const opaque = (el) => {
      if (/^(IMG|VIDEO|CANVAS)$/.test(el.tagName)) return true;
      const s = getComputedStyle(el);
      if (s.transform !== "none" || s.clipPath !== "none" || parseFloat(s.borderTopLeftRadius) > 0) return false;
      const m = /rgba?\(([^)]*)\)/.exec(s.backgroundColor);
      if (!m) return false;
      const p = m[1].split(/[,\s/]+/).filter(Boolean).map(Number);
      return p.length < 4 || p[3] >= 0.999;
    };
    const blocker = (h, target) => {
      let alpha = 1;
      const branch = [];
      for (let n = h; n && n !== root; n = up(n)) {
        if (within(n, target)) break; // a shared ancestor paints behind the target
        branch.push(n);
        alpha *= parseFloat(getComputedStyle(n).opacity) || 0;
      }
      if (alpha < 0.999) return null;
      return branch.find(opaque) || null;
    };
    for (const el of controls) {
      if (focused >= 150) break;
      if (el.tabIndex < 0 && !el.matches("a[href],button,input,select,textarea,summary")) continue;
      el.focus({ preventScroll: false });
      if (document.activeElement !== el && !(el.shadowRoot && el.shadowRoot.activeElement)) continue;
      focused++;
      const r = el.getBoundingClientRect();
      const vis = { left: Math.max(0, r.left), top: Math.max(0, r.top), right: Math.min(innerWidth, r.right), bottom: Math.min(innerHeight, r.bottom) };
      if (vis.right - vis.left < 1 || vis.bottom - vis.top < 1) continue;
      let shown = 0, sampled = 0;
      const by = new Map();
      for (const [x, y] of grid({ left: vis.left, top: vis.top, width: vis.right - vis.left, height: vis.bottom - vis.top }, 3)) {
        sampled++;
        const h = hit(x, y);
        if (!h || within(el, h)) { shown++; continue; }
        const b = blocker(h, el);
        if (!b) { shown++; continue; }
        by.set(b, (by.get(b) || 0) + 1);
      }
      if (!sampled || shown) continue;
      const cover = [...by.keys()].find((b) => {
        const br = b.getBoundingClientRect();
        return br.left <= vis.left + 0.5 && br.top <= vis.top + 0.5 && br.right >= vis.right - 0.5 && br.bottom >= vis.bottom - 0.5;
      });
      if (!cover) continue;
      obscuredTotal++;
      if (obscured.length < max) {
        const pos = getComputedStyle(cover).position;
        obscured.push({ ...named(el), by: named(cover), by_position: pos, box: boxNow(r) });
      }
    }
    if (before && before !== document.body && before.focus) before.focus({ preventScroll: true });
    else if (document.activeElement && document.activeElement.blur) document.activeElement.blur();
    scrollTo(sx, sy);
  }

  const font = parseFloat(getComputedStyle(root).fontSize);
  return {
    viewport: [innerWidth, innerHeight],
    root_font_px: font,
    controls: controls.length,
    overflow,
    reflow_exempt: reflowExempt,
    clipped,
    clipped_count: clippedTotal,
    overlaps,
    overlap_count: overlapTotal,
    small,
    small_count: smallTotal,
    focus_checked: !!focus,
    focused,
    obscured,
    obscured_count: obscuredTotal,
  };
}
