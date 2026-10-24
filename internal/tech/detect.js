// Runs the technology fingerprints against the loaded page, in the page's own realm. Called with (printsJSON), the
// fingerprints as JSON. Returns [{name, category, source, evidence}], one for each fingerprint that matched, with the
// first marker that did. A pattern that is not a valid regular expression is skipped.
(printsJSON) => {
  const prints = JSON.parse(printsJSON);
  const re = (p) => {
    try {
      return new RegExp(p, "i"); // nosemgrep: javascript.lang.security.audit.detect-non-literal-regexp.detect-non-literal-regexp -- the patterns are the engine's own or the user's webappanalyzer copy, never a page's, and the call has a time limit
    } catch (e) {
      return null;
    }
  };
  const clip = (s) => String(s).replace(/\s+/g, " ").slice(0, 120);
  const scripts = [...document.scripts].map((s) => s.src).filter(Boolean);
  const metas = {};
  for (const m of document.querySelectorAll("meta[name][content]")) {
    const k = m.getAttribute("name").toLowerCase();
    (metas[k] = metas[k] || []).push(m.getAttribute("content"));
  }
  let html = null;
  const markup = () => (html === null ? (html = document.documentElement.outerHTML.slice(0, 500000)) : html);
  let cookies = [];
  try {
    cookies = document.cookie.split(";").map((c) => c.split("=")[0].trim()).filter(Boolean);
  } catch (e) {
    cookies = [];
  }
  // Reads a dotted window property without changing anything. Prototype keys are never followed.
  const unsafe = new Set(["__proto__", "constructor", "prototype"]);
  const global = (path) => {
    try {
      const v = path.split(".").reduce((o, k) => (o !== null && o !== undefined && !unsafe.has(k) && k in Object(o) ? o[k] : undefined), window);
      return v !== undefined && v !== null;
    } catch (e) {
      return false;
    }
  };
  const out = [];
  for (const p of prints) {
    let ev = "";
    for (const g of p.globals || []) if (!ev && global(g)) ev = "window." + g;
    for (const s of p.scripts || []) {
      if (ev) break;
      const r = re(s);
      const hit = r && scripts.find((src) => r.test(src));
      if (hit) ev = "script " + clip(hit);
    }
    for (const [name, pat] of Object.entries(p.meta || {})) {
      if (ev) break;
      const r = re(pat);
      const hit = r && (metas[name.toLowerCase()] || []).find((c) => r.test(c));
      if (hit) ev = "meta " + name + "=" + clip(hit);
    }
    for (const sel of p.selectors || []) {
      if (ev) break;
      try {
        if (document.querySelector(sel)) ev = "element " + sel;
      } catch (e) {
        // not a selector this browser knows
      }
    }
    for (const d of p.dom || []) {
      if (ev) break;
      let els = [];
      try {
        els = [...document.querySelectorAll(d.selector)].slice(0, 50);
      } catch (e) {
        els = [];
      }
      const text = d.text !== undefined && d.text !== null && d.text !== "" ? re(d.text) : null;
      for (const el of els) {
        if (text && text.test(el.textContent || "")) {
          ev = "element " + d.selector + " with text " + clip(el.textContent);
          break;
        }
        for (const [a, pat] of Object.entries(d.attributes || {})) {
          const v = el.getAttribute(a);
          const r = re(pat);
          if (v !== null && r && r.test(v)) {
            ev = "element " + d.selector + " with " + a + "=" + clip(v);
            break;
          }
        }
        if (ev) break;
      }
    }
    for (const c of p.cookies || []) {
      if (ev) break;
      const r = re(c);
      const hit = r && cookies.find((n) => r.test(n));
      if (hit) ev = "cookie " + clip(hit);
    }
    for (const h of p.html || []) {
      if (ev) break;
      const r = re(h);
      const m = r && markup().match(r);
      if (m) ev = "markup " + clip(m[0]);
    }
    if (ev) out.push({ name: p.name, category: p.category, source: p.source || "dummyrun", evidence: ev });
  }
  return out;
}
