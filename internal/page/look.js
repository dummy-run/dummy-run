// What a persona sees: the page's title and headings, the text of its main
// content, and every visible control numbered so the persona can act on "@12"
// instead of guessing a selector. Numbers are reassigned on every look.
//
// Called as a function expression with (maxControls, textLimit, logLimit).
(maxControls, textLimit, logLimit) => {
  // Off-canvas elements (a closed drawer parked beyond the left or right edge) count as hidden.
  const visible = (el) => {
    const r = el.getBoundingClientRect();
    const s = getComputedStyle(el);
    const onCanvas = r.right > 0 && r.left < innerWidth;
    return r.width > 0 && r.height > 0 && onCanvas && s.visibility !== "hidden" && s.display !== "none";
  };
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim();
  const byId = (id) => (id ? document.getElementById(id) : null);
  const named = (el) =>
    squash(
      el.getAttribute("aria-label") ||
        (el.getAttribute("aria-labelledby") && byId(el.getAttribute("aria-labelledby").split(" ")[0])?.innerText) ||
        el.labels?.[0]?.innerText ||
        el.placeholder ||
        el.innerText ||
        el.value ||
        ""
    );
  const fallback = (el) =>
    squash(
      el.title ||
        el.querySelector("img[alt]")?.alt ||
        el.querySelector("svg title")?.textContent ||
        (el.href ? new URL(el.href, location.href).pathname : "") ||
        el.name ||
        ""
    );
  const label = (el) => {
    const n = named(el);
    const text = n || (fallback(el) ? "(no label) " + fallback(el) : "(no label)");
    return text.slice(0, 80);
  };
  document.querySelectorAll("[data-dummyrun-ref]").forEach((el) => el.removeAttribute("data-dummyrun-ref"));
  const selector = [
    "a[href]", "button", "input:not([type=hidden])", "select", "textarea", "summary",
    "[role=button]", "[role=link]", "[role=tab]", "[role=option]", "[role=menuitem]",
    "[role=checkbox]", "[role=switch]", "[role=radio]", "[contenteditable=true]", "[tabindex='0']",
  ].join(",");
  const controls = [];
  for (const el of document.querySelectorAll(selector)) {
    if (controls.length >= maxControls) break;
    if (!visible(el)) continue;
    const ref = String(controls.length + 1);
    el.setAttribute("data-dummyrun-ref", ref);
    const r = el.getBoundingClientRect();
    controls.push({
      ref,
      tag: el.tagName.toLowerCase(),
      type: el.type || el.getAttribute("role") || "",
      label: label(el),
      href: el.tagName === "A" ? el.getAttribute("href") || undefined : undefined,
      disabled: el.disabled || el.getAttribute("aria-disabled") === "true" || undefined,
      offscreen: r.bottom < 0 || r.top > innerHeight || undefined,
    });
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
  for (const el of document.body.querySelectorAll("*")) {
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
      const r = e.getBoundingClientRect();
      if (unseen(e) || r.bottom + scrollY <= 0 || s.position === "absolute" || s.position === "fixed" || parseFloat(s.fontSize) < 4 || /rgba\(.*,\s*0\)$/.test(s.color) || s.color === "transparent" || s.clipPath !== "none" || parseFloat(s.textIndent) < -100) return false;
    }
    return true;
  };
  const main = document.querySelector("main") || document.body;
  // The markers carry a random value for this look, so a page cannot forge or close one.
  const mark = String(Math.random()).slice(2) + String(Math.random()).slice(2);
  const open = "\uE000" + mark + "\uE001";
  const shut = "\uE002" + mark + "\uE003";
  const added = [];
  let quotes = 0;
  for (const el of document.body.querySelectorAll(quotedSel)) {
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
  const cut = new RegExp(open + "[\\s\\S]*?" + shut, "g");
  const clean = (t) => strip(t.replace(cut, " . "));
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
  };
}
