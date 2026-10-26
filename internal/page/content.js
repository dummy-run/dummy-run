// Checks on the words and images a page's authors wrote, measured in the browser with no model. They follow the
// kind of content checks that tools for editors make, written for Dummy Run. Called as a function expression with
// (max): the most nodes to return for each check.
//   vagueLink     a link whose whole name is a phrase such as "click here" or "read more"
//   urlLink       a link whose name is a long web address
//   newWindow     a link that opens a new tab or window and doesn't say so
//   documentLink  a link to a PDF or office document that doesn't say what kind of file it is
//   altFileName   an image whose alt text is a file name
//   altRedundant  an image whose alt text starts "image of" or "picture of"
//   altLong       an image whose alt text is more than 150 characters
//   fakeHeading   a short paragraph set entirely in bold, that looks like a heading but isn't one
//   allCaps       a run of text of more than 30 letters written in capitals
//   underline     underlined text that isn't a link
(max) => {
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim();
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
    for (let e = el, d = 0; e && e.nodeType === 1 && e !== document.body && e !== document.documentElement && d < 4; e = e.parentElement, d++) {
      const p = part(e);
      parts.unshift(p);
      if (p[0] === "#") break;
    }
    const s = parts.join(" > ") || el.tagName.toLowerCase();
    // A selector that matches more than one element falls back to the full path from the body.
    if (document.querySelectorAll(s).length === 1) return s;
    const full = [];
    for (let e = el; e && e.nodeType === 1 && e !== document.body && e !== document.documentElement; e = e.parentElement) full.unshift(part(e));
    return full.join(" > ");
  };
  const shown = (el) => {
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true, opacityProperty: true })) return false;
    const r = el.getBoundingClientRect();
    return r.width > 1 && r.height > 1;
  };
  const quote = (s, n) => {
    s = squash(s);
    return "\"" + (s.length > n ? s.slice(0, n - 3) + "..." : s) + "\"";
  };
  const out = { vagueLink: [], urlLink: [], newWindow: [], documentLink: [], altFileName: [], altRedundant: [], altLong: [], fakeHeading: [], allCaps: [], underline: [] };
  const push = (list, el, detail) => { if (list.length < max) list.push({ selector: selector(el), detail }); };

  // The phrase checks are in English, so they run only on a page in English or with no language set.
  const lang = (document.documentElement.getAttribute("lang") || "en").toLowerCase();
  const english = lang === "" || lang === "en" || lang.startsWith("en-");

  // A link's name from its attributes, or its text and the alt text of its images.
  const linkName = (a) => {
    const by = a.getAttribute("aria-labelledby");
    if (by) {
      const t = by.split(/\s+/).map((id) => document.getElementById(id)).filter(Boolean).map((n) => n.textContent).join(" ");
      if (squash(t)) return squash(t);
    }
    const label = a.getAttribute("aria-label");
    if (squash(label)) return squash(label);
    const bits = [a.innerText];
    for (const img of a.querySelectorAll("img[alt]")) bits.push(img.getAttribute("alt"));
    return squash(bits.join(" "));
  };
  const vague = /^(click here|click|here|tap here|read more|more|learn more|find out more|more info|more information|details|this link|link|this page|go|continue|see more|view more)$/;
  const documents = /\.(pdf|docx?|xlsx?|pptx?|odt|ods|odp|rtf|csv)$/i;
  const kinds = /\b(pdf|word|excel|powerpoint|spreadsheet|document|docx?|xlsx?|pptx?|odt|ods|odp|rtf|csv|download)\b/i;
  const warns = /\b(new (tab|window)|opens? in|external)\b/i;
  for (const a of document.querySelectorAll("a[href]")) {
    if (!shown(a)) continue;
    const name = linkName(a);
    const plain = name.toLowerCase().replace(/[.:!?…»›→>\s]+$/u, "").replace(/^[«‹←<\s]+/u, "");
    if (english && vague.test(plain)) push(out.vagueLink, a, "the link's name is " + quote(name, 60) + ", which says nothing about where it goes when heard on its own");
    if (/^(https?:\/\/|www\.)\S{30,}$/i.test(name)) push(out.urlLink, a, "the link's name is the address " + quote(name, 80));
    const title = a.getAttribute("title") || "";
    if (a.target === "_blank" && english && !warns.test(name + " " + title) && !a.querySelector("[aria-label*=\"new\" i], [title*=\"new\" i]")) {
      push(out.newWindow, a, "the link " + quote(name, 60) + " opens a new tab and doesn't say so");
    }
    let path = "";
    try { path = new URL(a.href, document.baseURI).pathname; } catch (e) { /* an address the browser can't read has no file type */ }
    if (documents.test(path) && english && !kinds.test(name + " " + title)) {
      push(out.documentLink, a, "the link " + quote(name, 60) + " goes to a ." + path.split(".").pop().toLowerCase() + " file and doesn't say what kind of file");
    }
  }

  const fileName = /^[\w ()-]+\.(jpe?g|png|gif|webp|avif|svg|bmp|tiff?)$|^(img|dsc|dscn|dcim|image|photo|screenshot|pxl)[-_ ]?\d{2,}/i;
  const redundant = /^(an? )?(image|picture|photo|photograph|graphic|icon) (of|showing)\b/i;
  for (const img of document.querySelectorAll("img[alt], input[type=image][alt], area[alt]")) {
    if (img.tagName !== "AREA" && !shown(img)) continue;
    const alt = squash(img.getAttribute("alt"));
    if (!alt) continue;
    if (fileName.test(alt)) push(out.altFileName, img, "the alt text is " + quote(alt, 80) + ", a file name");
    else if (english && redundant.test(alt)) push(out.altRedundant, img, "the alt text " + quote(alt, 60) + " starts by saying it is an image, which a screen reader says already");
    if (alt.length > 150) push(out.altLong, img, "the alt text has " + alt.length + " characters");
  }

  // A paragraph is a fake heading when all its text is bold, it is short, it has no full stop at the end and a
  // paragraph follows it.
  const bold = (el) => {
    const w = getComputedStyle(el).fontWeight;
    return w === "bold" || Number(w) >= 600;
  };
  const allBold = (p) => {
    const walk = document.createTreeWalker(p, NodeFilter.SHOW_TEXT);
    let any = false;
    for (let t = walk.nextNode(); t; t = walk.nextNode()) {
      if (!squash(t.textContent)) continue;
      any = true;
      if (!bold(t.parentElement)) return false;
    }
    return any;
  };
  for (const p of document.querySelectorAll("p")) {
    if (!shown(p) || p.closest("h1, h2, h3, h4, h5, h6, [role=heading], li, td, th, label, button, a, legend, caption, figcaption, nav, header, footer, form, dialog, [role=dialog], [role=alert]")) continue;
    const text = squash(p.innerText);
    if (!text || text.length > 120 || /[.,;:?!]$/.test(text) || p.querySelector("a, button, input")) continue;
    let next = p.nextElementSibling;
    while (next && !shown(next)) next = next.nextElementSibling;
    if (!next || !/^(P|UL|OL|DL|TABLE)$/.test(next.tagName)) continue;
    if (allBold(p)) push(out.fakeHeading, p, quote(text, 60) + " looks like a heading, but isn't marked up as one");
  }

  // Text in capitals: a block whose own text has more than 30 letters, all of them capitals as shown.
  const blocks = "p, li, h1, h2, h3, h4, h5, h6, td, th, blockquote, figcaption, dd, dt";
  for (const el of document.querySelectorAll(blocks)) {
    if (!shown(el) || el.querySelector(blocks)) continue;
    const text = squash(el.innerText);
    const letters = text.replace(/[^\p{L}]/gu, "");
    if (letters.length <= 30 || letters !== letters.toUpperCase() || letters === letters.toLowerCase()) continue;
    push(out.allCaps, el, quote(text, 60) + " is in capitals" + (getComputedStyle(el).textTransform === "uppercase" ? ", set by text-transform" : ""));
  }

  // Underlined text that isn't a link, so a reader may try to follow it.
  const seen = new Set();
  const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let t = walk.nextNode(); t; t = walk.nextNode()) {
    const el = t.parentElement;
    if (!el || seen.has(el) || !squash(t.textContent)) continue;
    seen.add(el);
    if (el.closest("a[href], button, [role=link], [role=button], [role=tab], abbr, ins, label, summary")) continue;
    let lined = null;
    for (let e = el; e && e !== document.body; e = e.parentElement) {
      if (getComputedStyle(e).textDecorationLine.includes("underline")) { lined = e; break; }
    }
    if (!lined || !shown(el)) continue;
    if (lined.closest("a[href], button, [role=link], [role=button], [role=tab], abbr, ins, label, summary")) continue;
    if (out.underline.some((n) => n.el === lined)) continue;
    if (out.underline.length < max) out.underline.push({ el: lined, selector: selector(lined), detail: quote(lined.innerText, 60) + " is underlined but isn't a link" });
  }
  out.underline = out.underline.map(({ selector, detail }) => ({ selector, detail }));
  return out;
}
