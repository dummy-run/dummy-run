// Journeys: finds the journeys a page offers and checks whether one has worked. It runs in the page's own realm,
// because a page's submit handlers and expanded menus live there. Called with (op, argJSON):
//   find     {}                 the journeys the page offers, at most one of each kind
//   guard    {}                 stops any form on the page from being sent, so a journey never submits
//   locate   {sel}              scrolls the element into view; {found, x, y, hittable}
//   present  {sel}              whether the element is on the page and visible, without scrolling
//   focused  {sel}              whether the element, or something inside it, has keyboard focus
//   check    {kind, sel, href}  whether the journey has worked: the success test
//   fields   {}                 the form fields a contact journey fills, as selectors
//   value    {sel}              an input's value
(op, argJSON) => {
  const arg = JSON.parse(argJSON || "{}");
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim();
  const visible = (el) => {
    if (!el || !el.isConnected) return false;
    const r = el.getBoundingClientRect();
    if (r.width < 1 || r.height < 1) return false;
    const s = getComputedStyle(el);
    if (s.visibility === "hidden" || s.display === "none") return false;
    return !el.closest("[aria-hidden=true], [inert], details:not([open]) > :not(summary)");
  };
  const part = (el) => {
    if (el.id && document.querySelectorAll("#" + CSS.escape(el.id)).length === 1) return "#" + CSS.escape(el.id);
    let s = el.tagName.toLowerCase();
    const p = el.parentElement;
    if (p) {
      const same = [...p.children].filter((c) => c.tagName === el.tagName);
      if (same.length > 1) s += ":nth-of-type(" + (same.indexOf(el) + 1) + ")";
    }
    return s;
  };
  const selector = (el) => {
    const parts = [];
    for (let e = el; e && e.nodeType === 1 && e !== document.documentElement; e = e.parentElement) {
      const p = part(e);
      parts.unshift(p);
      if (p[0] === "#") break;
    }
    const sel = parts.join(" > ");
    try {
      return document.querySelectorAll(sel).length === 1 ? sel : "";
    } catch (e) {
      return "";
    }
  };
  const name = (el) => {
    const label = el.labels && el.labels.length ? el.labels[0].textContent : "";
    return squash(el.getAttribute("aria-label") || label || el.getAttribute("placeholder") || el.textContent || el.getAttribute("title") || el.value || "").slice(0, 80);
  };
  const same = (a, b) => {
    try {
      const x = new URL(a, location.href), y = new URL(b, location.href);
      x.hash = "";
      y.hash = "";
      return x.href.replace(/\/$/, "") === y.href.replace(/\/$/, "");
    } catch (e) {
      return false;
    }
  };
  // What a page reached by a journey must have.
  const arrived = {
    signup: () => !!document.querySelector('input[type=email], input[type=password], input[autocomplete~=email], input[autocomplete~="new-password"]'),
    basket: () => /\b(basket|cart|bag|trolley)\b/i.test(document.title + " " + [...document.querySelectorAll("h1, h2")].map((h) => h.textContent).join(" ")),
    booking: () => !!document.querySelector('input[type=date], input[type=time], input[type=datetime-local], [role=grid], iframe[src*="calendly"], iframe[src*="opentable"], iframe[src*="resdiary"], iframe[src*="acuity"], iframe[src*="simplybook"], dialog[open] form, [role=dialog] form'),
    contact: () => [...document.querySelectorAll("form")].some((f) => visible(f) && f.querySelector('textarea, input[type=email]')),
  };
  // schema.org data the page publishes as JSON-LD.
  const schema = () => {
    const out = [];
    const walk = (v) => {
      if (Array.isArray(v)) return v.forEach(walk);
      if (!v || typeof v !== "object") return;
      out.push(v);
      for (const k of Object.keys(v)) if (typeof v[k] === "object") walk(v[k]);
    };
    for (const s of document.querySelectorAll('script[type="application/ld+json"]')) {
      try {
        walk(JSON.parse(s.textContent));
      } catch (e) {
        // not JSON
      }
    }
    return out;
  };
  const types = (o) => [].concat(o["@type"] || []).map(String);

  if (op === "find") {
    const out = [];
    const first = (kind, els, source, href) => {
      for (const el of els) {
        if (!visible(el)) continue;
        // A journey starts from a link, a button or a field, never from a submit button.
        if (el.matches('[type=submit], form button:not([type])')) continue;
        const sel = selector(el);
        if (!sel) continue;
        out.push({ kind, selector: sel, label: name(el), href: href === undefined ? (el.href || "") : href, source });
        return true;
      }
      return false;
    };
    const all = (q) => [...document.querySelectorAll(q)];
    const byText = (q, re) => all(q).filter((el) => re.test(name(el)));
    const objects = schema();
    const site = objects.find((o) => types(o).some((t) => /WebSite/.test(t)) && o.potentialAction);
    const searchFromSchema = !!(site && [].concat(site.potentialAction).some((a) => types(a).some((t) => /SearchAction/.test(t))));
    first("search", all('input[type=search], form[role=search] input:not([type=hidden]), search input:not([type=hidden]), [role=searchbox], input[name=q], input[name=s], input[name=query], input[name=search]'),
      searchFromSchema ? "schema.org" : "dom", "");
    first("menu", all('header button[aria-expanded=false], nav button[aria-expanded=false], button[aria-expanded=false][aria-controls]').filter((b) => /menu|navigation|nav\b/i.test(name(b) + " " + (b.getAttribute("aria-controls") || ""))), "dom", "");
    first("signup", byText("a[href], button", /^(sign up|sign-up|register|create (an |your )?account|join( now| us)?)\b/i), "dom");
    // A link that adds to the basket changes it when followed, so only a link to the basket itself starts the journey.
    const adds = (a) => /\badd\b/i.test(name(a)) || /[?&](add|add-to-cart|add_to_cart|add-to-basket)=|\/(cart|basket)\/add\b/i.test(a.getAttribute("href") || "");
    first("basket", [...byText("a[href]", /\b(basket|cart|bag|trolley)\b/i), ...all('a[href*="/cart"], a[href*="/basket"], a[href*="/bag"]')].filter((a) => !adds(a)), "dom");
    const reserve = objects.find((o) => [].concat(o.potentialAction || []).some((a) => types(a).some((t) => /ReserveAction|ReservationAction|ScheduleAction/.test(t))));
    first("booking", byText("a[href], button", /\b(book( now| a| an| online| your)?|reserve|reservation|appointment)\b/i), reserve ? "schema.org" : "dom");
    first("contact", [...byText("a[href]", /^(contact( us)?|get in touch)\b/i), ...all('a[href*="/contact"]')], "dom");
    return out;
  }

  if (op === "guard") {
    if (!window.__dummyrunNoSubmit) {
      const stop = (e) => {
        e.preventDefault();
        e.stopImmediatePropagation();
      };
      document.addEventListener("submit", stop, true);
      Object.defineProperty(window, "__dummyrunNoSubmit", { value: true });
      // A form's own submit() skips the event, so it is stopped too.
      try {
        HTMLFormElement.prototype.submit = function () {};
        HTMLFormElement.prototype.requestSubmit = function () {};
      } catch (e) {
        // a page that froze its prototypes keeps them
      }
    }
    return true;
  }

  const el = arg.sel ? document.querySelector(arg.sel) : null;
  if (op === "locate") {
    if (!el || !visible(el)) return { found: false };
    el.scrollIntoView({ block: "center", inline: "center" });
    const r = el.getBoundingClientRect();
    const x = r.left + r.width / 2, y = r.top + r.height / 2;
    const top = document.elementFromPoint(x, y);
    return { found: true, x, y, hittable: !!top && (top === el || el.contains(top) || top.contains(el)) };
  }
  if (op === "present") return !!el && visible(el);
  if (op === "focused") {
    let a = document.activeElement;
    while (a && a.shadowRoot && a.shadowRoot.activeElement) a = a.shadowRoot.activeElement;
    return !!el && !!a && (a === el || el.contains(a));
  }
  if (op === "value") return el ? String(el.value || "") : "";
  if (op === "check") {
    switch (arg.kind) {
      case "menu": {
        const b = el;
        if (!b) return false;
        if (b.getAttribute("aria-expanded") === "true") return true;
        const t = document.getElementById(b.getAttribute("aria-controls") || "");
        return !!t && visible(t) && t.querySelectorAll("a[href]").length > 0;
      }
      case "search":
        return !!el && (el.value || "").includes(arg.typed || "");
      default: {
        const done = arrived[arg.kind] ? arrived[arg.kind]() : false;
        if (arg.href) return same(location.href, arg.href) && done;
        return done;
      }
    }
  }
  if (op === "fields") {
    const form = [...document.querySelectorAll("form")].find((f) => visible(f) && f.querySelector("textarea, input[type=email]"));
    if (!form) return [];
    return [...form.querySelectorAll('input:not([type]), input[type=text], input[type=email], input[type=tel], textarea')]
      .filter(visible).slice(0, 6).map(selector).filter(Boolean);
  }
  return null;
}
