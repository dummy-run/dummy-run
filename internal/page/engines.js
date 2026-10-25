// Runs IBM Equal Access, HTML_CodeSniffer, Siteimprove Alfa and QualWeb on the loaded page, in the sweep's sandbox, and
// matches their failures to elements. No model is involved: each result is an engine's own rule outcome.
// Called as a function expression with (axeJSON, criteriaJSON, max):
//   axeJSON       [{sc: ["1.1.1"], selector: "img"}] as JSON: axe's failing nodes, so agreement can be counted
//   criteriaJSON  the WCAG criteria to keep as JSON, such as ["1.1.1", ...]; anything else, such as AAA, is dropped
//   max       the most groups to return
// Returns {ran, errors, axe, groups, omitted}:
//   ran     the engines that finished
//   errors  {engine: message} for an engine that threw
//   axe     for each of axeNodes, the other engines that flagged the same element for a shared criterion
//   groups  the other failures axe did not report, one per element and criterion, the groups more engines agree on first
//   omitted how many groups were left out to keep within max
async (axeJSON, criteriaJSON, max) => {
  const axeNodes = JSON.parse(axeJSON);
  const keep = new Set(JSON.parse(criteriaJSON));
  // Control characters from page text could rewrite a terminal line when the summary prints a message.
  const squash = (s) => (s || "").replace(/[\u0000-\u001f\u007f-\u009f]/g, " ").replace(/\s+/g, " ").trim();
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
    return parts.join(" > ") || el.tagName.toLowerCase();
  };
  // The page itself: a missing title or language is reported on html, head, body or the document by different engines.
  const PAGE = document.documentElement;
  const element = (n) => {
    if (!n) return null;
    if (n.nodeType === 9) return PAGE;
    if (n.nodeType !== 1) n = n.parentElement;
    if (!n) return null;
    if (["HTML", "HEAD", "BODY", "TITLE", "META"].includes(n.tagName) || n.closest("head")) return PAGE;
    return n;
  };
  const byPath = (p) => {
    if (!p || p === "/") return PAGE;
    try {
      return document.evaluate(p, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
    } catch (e) {
      return null;
    }
  };
  const ran = [];
  const errors = {};
  const items = []; // {engine, rule, sc, el, message, help}
  // Text no one can see, such as a button label pushed off the page with text-indent, text set at font-size 0 or
  // visually hidden text clipped to a pixel, has no contrast a person sees. Engines that measure it anyway report a
  // false contrast failure. Text counts as seen when more than a pixel square of it is left on the page once every
  // element that hides its overflow has clipped it. Text that spills out of its element is still seen.
  const seen = (q, t) => {
    let l = q.left, r = q.right, top = q.top, bot = q.bottom;
    if (r + scrollX <= 0 || bot + scrollY <= 0) return false; // off the page to the left or top
    for (let a = t.parentElement; a && a !== document.documentElement; a = a.parentElement) {
      const cs = getComputedStyle(a);
      if (cs.overflowX === "visible" && cs.overflowY === "visible") continue;
      const b = a.getBoundingClientRect();
      l = Math.max(l, b.left);
      r = Math.min(r, b.right);
      top = Math.max(top, b.top);
      bot = Math.min(bot, b.bottom);
    }
    return r - l > 1 && bot - top > 1;
  };
  const unpainted = (el) => {
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    let any = false;
    for (let t = walker.nextNode(), n = 0; t && n < 200; t = walker.nextNode(), n++) {
      if (!t.data.trim()) continue;
      any = true;
      const r = document.createRange();
      r.selectNodeContents(t);
      for (const q of r.getClientRects()) {
        if (seen(q, t)) return false;
      }
    }
    return any;
  };
  const add = (engine, rule, sc, node, message, help) => {
    sc = [...new Set(sc)].filter((c) => keep.has(c));
    const el = element(node);
    if (el && el !== PAGE && sc.includes("1.4.3") && unpainted(el)) sc = sc.filter((c) => c !== "1.4.3");
    if (!sc.length || !el) return;
    items.push({ engine, rule, sc, el, message: squash(message).slice(0, 200), help });
  };

  try {
    const checker = new ace.Checker();
    const map = {};
    for (const g of checker.getGuidelines()) {
      if (g.id !== "WCAG_2_2") continue;
      for (const cp of g.checkpoints) for (const r of cp.rules || []) (map[r.id] = map[r.id] || []).push(cp.num);
    }
    const rep = await checker.check(document, ["WCAG_2_2"]);
    for (const r of rep.results) {
      if (r.value[0] !== "VIOLATION" || r.value[1] !== "FAIL") continue;
      add("ibm", r.ruleId, map[r.ruleId] || [], byPath(r.path && r.path.dom), r.message,
        "https://able.ibm.com/rules/archives/latest/doc/en-US/" + r.ruleId + ".html");
    }
    ran.push("ibm");
  } catch (e) {
    errors.ibm = squash(String(e && e.message || e)).slice(0, 200);
  }

  try {
    const msgs = await new Promise((resolve, reject) => {
      HTMLCS.process("WCAG2AA", document, () => resolve(HTMLCS.getMessages()), reject, "en");
    });
    for (const m of msgs) {
      if (m.type !== HTMLCS.ERROR) continue;
      const hit = /Guideline\d+_\d+\.(\d+)_(\d+)_(\d+)\./.exec(m.code);
      if (!hit) continue;
      add("htmlcs", m.code.split(".").slice(4).join("."), [hit[1] + "." + hit[2] + "." + hit[3]], m.element, m.msg, "");
    }
    ran.push("htmlcs");
  } catch (e) {
    errors.htmlcs = squash(String(e && e.message || e)).slice(0, 200);
  }

  try {
    for (const o of await dummyrunAlfa()) {
      add("alfa", o.rule, o.sc, byPath(o.path), "", "https://alfa.siteimprove.com/rules/" + o.rule);
    }
    ran.push("alfa");
  } catch (e) {
    errors.alfa = squash(String(e && e.message || e)).slice(0, 200);
  }

  try {
    if (typeof ACTRulesRunner !== "function" || typeof WCAGTechniquesRunner !== "function") throw new Error("did not load");
    // A technique's page on the W3C site, from its ID: G1 is general, H44 html, ARIA6 aria and so on.
    const techniqueDirs = { G: "general", H: "html", ARIA: "aria", C: "css", F: "failures", SCR: "client-side-script", SVR: "server-side-script", SM: "smil", T: "text", PDF: "pdf" };
    const help = (kind, mapping) => {
      if (!mapping) return "";
      if (kind === "act") return "https://www.w3.org/WAI/standards-guidelines/act/rules/" + encodeURIComponent(mapping) + "/";
      const m = /^([A-Z]+)\d+$/.exec(mapping);
      return m && techniqueDirs[m[1]] ? "https://www.w3.org/WAI/WCAG22/Techniques/" + techniqueDirs[m[1]] + "/" + mapping : "";
    };
    const byPointer = (p) => {
      try {
        return document.querySelector(p);
      } catch (e) {
        return null;
      }
    };
    // The ACT rule that needs the page at another window size (QW-ACT-R40) is left out, so the window never changes.
    const reports = [
      ["act", new ACTRulesRunner({}, "en").configure({}).test({}).getReport()],
      ["wcag", new WCAGTechniquesRunner({}, "en").configure({}).test({}).getReport()],
    ];
    for (const [kind, rep] of reports) {
      for (const [code, a] of Object.entries(rep.assertions || {})) {
        if (!a || !a.metadata || a.metadata.outcome !== "failed") continue;
        const sc = (a.metadata["success-criteria"] || []).map((c) => c.name);
        for (const r of a.results || []) {
          if (r.verdict !== "failed") continue;
          // A result with no element is about the page as a whole.
          const els = r.elements && r.elements.length ? r.elements : [{}];
          for (const e of els) add("qualweb", code, sc, e.pointer ? byPointer(e.pointer) : document, r.description, help(kind, a.mapping));
        }
      }
    }
    ran.push("qualweb");
  } catch (e) {
    errors.qualweb = squash(String(e && e.message || e)).slice(0, 200);
  }

  const used = new Set();
  const axe = axeNodes.map((n) => {
    let el = null;
    if (!n.selector.includes(" >> ")) {
      try {
        el = element(document.querySelector(n.selector));
      } catch (e) {
        el = null;
      }
    }
    // Engines are counted per criterion, so IBM on 1.1.1 and Alfa on 1.3.1 never add up to agreement.
    const agree = new Map(n.sc.map((c) => [c, new Set()]));
    if (el) {
      items.forEach((it, i) => {
        if (it.el !== el) return;
        // Only the criteria axe shares are accounted for; the item's other criteria still group below.
        for (const c of it.sc) {
          if (!agree.has(c)) continue;
          agree.get(c).add(it.engine);
          used.add(i + ":" + c);
        }
      });
    }
    let best = new Set();
    for (const s of agree.values()) if (s.size > best.size) best = s;
    return [...best].sort();
  });

  // Group what is left by element and exact criterion, so agreement means two engines report the same criterion.
  const groups = [];
  const index = new Map();
  items.forEach((it, i) => {
    for (const c of it.sc) {
      if (used.has(i + ":" + c)) continue;
      let byCriterion = index.get(it.el);
      if (!byCriterion) index.set(it.el, (byCriterion = new Map()));
      let g = byCriterion.get(c);
      if (!g) {
        g = { el: it.el, sc: c, hits: [] };
        groups.push(g);
        byCriterion.set(c, g);
      }
      g.hits.push({ engine: it.engine, rule: it.rule, message: it.message, help: it.help });
    }
  });
  const engines = (g) => new Set(g.hits.map((h) => h.engine)).size;
  // A stable sort keeps discovery order among groups with the same number of engines.
  const ranked = groups.map((g, i) => ({ g, i, n: engines(g) })).sort((a, b) => b.n - a.n || a.i - b.i).map((x) => x.g);
  const out = ranked.slice(0, max).map((g) => ({
    selector: g.el === PAGE ? "html" : selector(g.el),
    sc: [g.sc],
    engines: [...new Set(g.hits.map((h) => h.engine))].sort(),
    hits: g.hits,
  }));
  return { ran, errors, axe, groups: out, omitted: Math.max(ranked.length - max, 0) };
}
