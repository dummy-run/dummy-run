// What a persona sees: the page's title and headings, the text of its main
// content, and every visible control numbered so the persona can act on "@12"
// instead of guessing a selector. Numbers are reassigned on every look.
//
// Called as a function expression with (maxControls, textLimit, logLimit, mark). mark is a random value
// made by the engine for this look.
(maxControls, textLimit, logLimit, mark) => {
  const contentType = document.contentType || "";
  // A feed, a PDF, JSON or an image is a file, not a web page. There are no controls to number and no
  // page text to read, and its text is not sent to the model.
  if (!/^(text\/html|application\/xhtml\+xml)/i.test(contentType)) {
    return {
      url: location.href, title: "", viewport: [innerWidth, innerHeight], device_pixel_ratio: devicePixelRatio,
      scroll: [0, 0], headings: [], alerts: [], text: "", log_text: "", controls: [], hidden: [], scan_text: "",
      content_type: contentType,
    };
  }
/*NAMES*/
  // On screen in the layout, for the page text: the same rule the control list uses.
  const visible = drRendered;
  const squash = drSquash;
  // A hint that helps the persona tell nameless controls apart. It is not part of the name.
  const hint = (el) => {
    let path = "";
    try {
      path = el.href ? new URL(el.href, location.href).pathname : "";
    } catch (e) {
      path = el.getAttribute("href") || "";
    }
    return squash(path || el.getAttribute("name") || "");
  };
  const label = (el, role) => {
    const name = drName(el, role);
    if (name) return drClip(name, 80);
    return hint(el) ? "(no name) " + drClip(hint(el), 60) : "(no name)";
  };
  document.querySelectorAll("[data-dummyrun-ref]").forEach((el) => el.removeAttribute("data-dummyrun-ref"));
  // Only controls a screen reader user can reach are numbered. The rest are not on the page for them.
  const controls = [];
  drAxeSetup();
  try {
    for (const el of document.querySelectorAll(drInteractive)) {
      if (controls.length >= maxControls) break;
      if (!drExposed(el)) continue;
      const ref = String(controls.length + 1);
      el.setAttribute("data-dummyrun-ref", ref);
      const r = el.getBoundingClientRect();
      const role = drRole(el);
      controls.push({
        ref,
        tag: el.tagName.toLowerCase(),
        type: el.type || el.getAttribute("role") || "",
        role,
        label: label(el, role),
      href: el.tagName === "A" ? el.getAttribute("href") || undefined : undefined,
      disabled: el.matches(":disabled") || el.getAttribute("aria-disabled") === "true" || undefined,
        offscreen: r.bottom < 0 || r.top > innerHeight || undefined,
      });
    }
  } finally {
    drAxeTeardown();
  }
  const texts = (sel) =>
    [...document.querySelectorAll(sel)].filter(visible).map((e) => squash(e.innerText)).filter(Boolean);
  // Text and attribute values a person does not see. They are not sent to the model. The engine
  // scans them for text that tries to instruct it, and keeps them in steps.jsonl for the reviewer.
  const hidden = new Set();
  let hiddenChars = 0;
  const keepHidden = (s) => {
    s = squash(s).slice(0, 300);
    if (s && hiddenChars < 3000 && !hidden.has(s)) {
      hidden.add(s);
      hiddenChars += s.length;
    }
  };
  const unseen = (el) => {
    const r = el.getBoundingClientRect();
    const s = getComputedStyle(el);
    return !visible(el) || r.width < 2 || r.height < 2 || s.opacity === "0" || parseFloat(s.fontSize) === 0 || el.getAttribute("aria-hidden") === "true";
  };
  let scanned = 0;
  for (const el of (document.body || document.documentElement).querySelectorAll("*")) {
    if (++scanned > 5000) break;
    for (const attr of ["aria-label", "aria-description", "title", "alt", "placeholder"]) keepHidden(el.getAttribute(attr));
    if (/^(SCRIPT|STYLE|NOSCRIPT|TEMPLATE)$/.test(el.tagName)) continue;
    const own = [...el.childNodes].filter((n) => n.nodeType === 3).map((n) => n.nodeValue).join(" ");
    if (squash(own) && unseen(el)) keepHidden(own);
  }
  const walker = document.createTreeWalker(document, NodeFilter.SHOW_COMMENT);
  for (let n = walker.nextNode(), c = 0; n && c < 200; n = walker.nextNode(), c++) keepHidden(n.nodeValue);
  // Quoted examples: code, preformatted text, keys, sample output, block quotes and short quotes that a
  // person can see in the normal flow of the page. A docs page that shows an example of an attack in a
  // code block is content, so the injection scan leaves these out. Anything hidden, off screen or
  // transparent does not count, nor does a quote inside such an element. The scan text is the page text
  // with each such quote cut out where it stands, so the same words in ordinary prose still show.
  const quotedSel = "code, pre, kbd, samp, blockquote, q";
  const inFlow = (el) => {
    for (let e = el; e && e !== document.body; e = e.parentElement) {
      const s = getComputedStyle(e);
      if (s.display === "contents") {
        // No box of its own, so only these two can hide what is inside.
        if (e.getAttribute("aria-hidden") === "true" || s.opacity === "0") return false;
        continue;
      }
      const r = e.getBoundingClientRect();
      if (unseen(e) || r.bottom + scrollY <= 0 || s.position === "absolute" || s.position === "fixed" || parseFloat(s.fontSize) < 4 || /rgba\(.*,\s*0\)$/.test(s.color) || s.color === "transparent" || s.clipPath !== "none" || parseFloat(s.textIndent) < -100) return false;
    }
    return true;
  };
  const main = document.querySelector("main") || document.body;
  // The markers carry a random value that the engine made for this look. The page cannot predict it,
  // so it cannot forge or close one.
  const open = "\uE000" + mark + "\uE001";
  const shut = "\uE002" + mark + "\uE003";
  const added = [];
  let quotes = 0;
  for (const el of (document.body || document.documentElement).querySelectorAll(quotedSel)) {
    if (quotes >= 500) break;
    if (el.parentElement && el.parentElement.closest(quotedSel)) continue;
    if (!inFlow(el) || !el.parentNode) continue;
    quotes++;
    const before = document.createTextNode(open);
    const after = document.createTextNode(shut);
    el.parentNode.insertBefore(before, el);
    el.parentNode.insertBefore(after, el.nextSibling);
    added.push(before, after);
  }
  let body = "";
  let headings = [];
  let alerts = [];
  try {
    body = (main.innerText || "").replace(/\n{3,}/g, "\n\n").trim();
    headings = texts("h1, h2").slice(0, 10);
    alerts = texts("[role=alert], [role=status], [aria-live]").slice(0, 5);
  } finally {
    for (const n of added) n.remove();
  }
  const strip = (t) => t.split(open).join("").split(shut).join("");
  const cut = (t) => {
    let out = "";
    for (;;) {
      const i = t.indexOf(open);
      const j = i < 0 ? -1 : t.indexOf(shut, i + open.length);
      if (j < 0) return out + t;
      out += t.slice(0, i) + " . ";
      t = t.slice(j + shut.length);
    }
  };
  const clean = (t) => strip(cut(t));
  const scanText = [clean(body).slice(0, textLimit), ...headings.map(clean), ...alerts.map(clean)].join("\n");
  body = strip(body);
  headings = headings.map(strip);
  alerts = alerts.map(strip);
  return {
    url: location.href,
    title: document.title,
    viewport: [innerWidth, innerHeight],
    device_pixel_ratio: devicePixelRatio,
    scroll: [Math.round(scrollY), Math.round(document.documentElement.scrollHeight)],
    headings,
    alerts,
    text: body.slice(0, textLimit),
    log_text: body.slice(0, logLimit),
    controls,
    hidden: [...hidden],
    scan_text: scanText,
    content_type: contentType,
  };
}
