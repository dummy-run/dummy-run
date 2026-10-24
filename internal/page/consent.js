// Finds a cookie banner and answers it with DuckDuckGo autoconsent, choosing to opt out. No model is involved.
// Called as a function expression with (rulesJSON, waitMs). The engine puts the autoconsent.playwright.js bundle in
// place of the AUTOCONSENT marker below, so it is part of the call and a content security policy cannot block it.
//   rulesJSON  autoconsent's compact rule set as JSON
//   waitMs     how long to wait for autoconsent to finish once it finds a banner
// Before autoconsent clicks anything, its own heuristics read the banners on the page and sort each button into
// reject, accept, settings, acknowledge or other. Returns {popups, cmp, found, opted_out, done, settled, failed, cleared, error, steps}.
async (rulesJSON, waitMs) => {
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
    return parts.join(" > ") || el.tagName.toLowerCase();
  };
  const st = { popups: [], cmp: "", found: false, opted_out: false, done: false, settled: false, failed: false, cleared: false, error: "", steps: [] };
  let answered = false;
  // autoconsent's own progress: "nothingDetected" means it has finished looking and found no banner.
  let lifecycle = "";
  const els = [];
  // Reads the banners on the page and sorts each button, before anything is clicked.
  const look = () => {
    for (const p of window.autoconsentHeuristics.getPotentialPopups(1500)) {
      const buttons = p.buttons.slice(0, 12).map((b) => ({ text: squash(b.text).slice(0, 60), kind: b.regexClassification || "other" }));
      // A menu or header with cookie words in it can look like a popup. Only one with a consent button counts.
      if (!buttons.some((b) => b.kind !== "other")) continue;
      els.push(p.element);
      st.popups.push({ selector: selector(p.element), text: squash(p.text).slice(0, 160), buttons });
    }
  };
  if (window.__dummyrunConsent) return window.__dummyrunConsent.state;
  const rules = { compact: JSON.parse(rulesJSON) };
  const config = {
    enabled: true, autoAction: "optOut", disabledCmps: [], enablePrehide: false, enableCosmeticRules: true,
    enableGeneratedRules: true, detectRetries: 6, isMainWorld: true, prehideTimeout: 2000,
    enableHeuristicDetection: true, heuristicMode: "reject", logs: { errors: false },
  };
  const reply = (m) => setTimeout(() => window.autoconsentReceiveMessage && window.autoconsentReceiveMessage(m), 0);
  window.__dummyrunConsent = { state: st };
  window.autoconsentSendMessage = (m) => {
    if (!m || typeof m !== "object") return;
    if (m.type === "report" && m.state && m.state.lifecycle) lifecycle = m.state.lifecycle;
    if (m.type !== "report" && st.steps.length < 30) st.steps.push(m.type + (m.cmp ? " " + m.cmp : "") + ("result" in m ? " " + m.result : ""));
    switch (m.type) {
      case "init":
        reply({ type: "initResp", config, rules });
        break;
      case "eval": {
        // A snippet the page's content security policy blocks counts as false, as it does in autoconsent's own tests.
        let result = false;
        try {
          result = !!(0, eval)(m.code);
        } catch (e) {
          result = false;
        }
        reply({ type: "evalResp", id: m.id, result });
        break;
      }
      case "cmpDetected":
        st.cmp = m.cmp || st.cmp;
        break;
      case "popupFound":
        st.found = true;
        st.cmp = m.cmp || st.cmp;
        // A banner that appeared after the first look is read now: autoconsent sends this before it clicks.
        if (!els.length) {
          try {
            look();
          } catch (e) {
            st.error = String(e && e.message || e).slice(0, 200);
          }
        }
        break;
      case "optOutResult":
        st.opted_out = !!m.result;
        answered = true;
        break;
      case "autoconsentDone":
        st.done = true;
        break;
      case "autoconsentError":
        st.error = squash(JSON.stringify(m.details || "")).slice(0, 200);
        break;
    }
  };
  try {
    /*AUTOCONSENT*/
    // The bundle starts on load and waits for its rules, which arrive on the next tick, so nothing is clicked yet.
    look();
  } catch (e) {
    st.error = String(e && e.message || e).slice(0, 200);
    st.failed = true;
    return st;
  }
  const started = Date.now();
  let doneAt = 0;
  // With no banner, the wait ends when autoconsent has finished looking, so a banner that turns up late is still
  // answered here and never clicked during the checks that follow. The opt-out result can follow the done message,
  // so it gets a moment more.
  // Between tries autoconsent waits for the page to change, so on a still page it would wait a minute. A harmless
  // attribute on the root lets each try go ahead, and is removed afterwards.
  let tick = 0;
  while (Date.now() - started < waitMs) {
    await new Promise((r) => setTimeout(r, 250));
    if (lifecycle === "started") document.documentElement.setAttribute("data-dummyrun-consent", String(tick++));
    if (st.done && !doneAt) doneAt = Date.now();
    if (doneAt && (answered || Date.now() - doneAt > 1000)) break;
    if (!st.popups.length && !st.cmp && lifecycle === "nothingDetected") break;
  }
  // Said, not hidden: autoconsent waits for the page to change before it looks again, so on a still page it can
  // be looking when the wait ends.
  st.settled = st.done || lifecycle === "nothingDetected";
  document.documentElement.removeAttribute("data-dummyrun-consent");
  // Measured, not reported by autoconsent: every banner it found is gone or hidden now.
  // A banner often slides or fades away, so give it a moment, and count one moved out of the window as gone.
  if (els.length) await new Promise((r) => setTimeout(r, 600));
  const hidden = (el) => {
    if (!el.isConnected) return true;
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true, opacityProperty: true })) return true;
    const r = el.getBoundingClientRect();
    return r.width < 1 || r.height < 1 || r.bottom <= 0 || r.top >= innerHeight || r.right <= 0 || r.left >= innerWidth;
  };
  st.cleared = els.length > 0 && els.every(hidden);
  return st;
}
