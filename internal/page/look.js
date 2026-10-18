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
  const main = document.querySelector("main") || document.body;
  const body = (main.innerText || "").replace(/\n{3,}/g, "\n\n").trim();
  return {
    url: location.href,
    title: document.title,
    viewport: [innerWidth, innerHeight],
    scroll: [Math.round(scrollY), Math.round(document.documentElement.scrollHeight)],
    headings: texts("h1, h2").slice(0, 10),
    alerts: texts("[role=alert], [role=status], [aria-live]").slice(0, 5),
    text: body.slice(0, textLimit),
    log_text: body.slice(0, logLimit),
    controls,
  };
}
