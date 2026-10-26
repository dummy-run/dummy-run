// Checks on the page's markup that axe's default rules leave out, measured in the browser with no model.
// Called as a function expression with (max): the most nodes to return for each rule.
//   labelInName   a control whose visible words are not part of its accessible name (WCAG 2.5.3)
//   autocomplete  a field that asks for the person's own details with no autocomplete value that names a purpose (WCAG 1.3.5)
//   ariaFeatures  each role and aria-* attribute shown elements use, with the first element and how many use it
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
    return parts.join(" > ") || el.tagName.toLowerCase();
  };
  const shown = (el) => {
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true, opacityProperty: true })) return false;
    const r = el.getBoundingClientRect();
    return r.width > 1 && r.height > 1;
  };
  // Letters and digits only, lower case, so punctuation and spacing do not count.
  const words = (s) => squash(s).toLowerCase().replace(/[^\p{L}\p{N}]+/gu, " ").trim();
  // The autofill field names from the HTML standard.
  const fields = new Set(("name honorific-prefix given-name additional-name family-name honorific-suffix nickname username " +
    "new-password current-password one-time-code organization-title organization street-address address-line1 address-line2 " +
    "address-line3 address-level4 address-level3 address-level2 address-level1 country country-name postal-code cc-name " +
    "cc-given-name cc-additional-name cc-family-name cc-number cc-exp cc-exp-month cc-exp-year cc-csc cc-type " +
    "transaction-currency transaction-amount language bday bday-day bday-month bday-year sex url photo tel tel-country-code " +
    "tel-national tel-area-code tel-local tel-local-prefix tel-local-suffix tel-extension email impp").split(" "));
  // Whether an autocomplete value names a purpose, by the HTML standard's grammar: an optional "section-*", an optional
  // "shipping" or "billing", an optional contact kind before a phone, email or IMPP field, the field name, then an
  // optional "webauthn".
  const names = (value) => {
    const t = value.split(/\s+/).filter(Boolean);
    if (t[t.length - 1] === "webauthn") t.pop();
    const field = t.pop();
    if (!fields.has(field)) return false;
    let i = 0;
    if (t[i] && t[i].startsWith("section-")) i++;
    if (t[i] === "shipping" || t[i] === "billing") i++;
    if (["home", "work", "mobile", "fax", "pager"].includes(t[i]) && /^(tel|email|impp)/.test(field)) i++;
    return i === t.length;
  };
  const out = { labelInName: [], autocomplete: [], ariaFeatures: [] };
  const push = (list, node) => { if (list.length < max) list.push(node); };

  const nameFromAttrs = (el) => {
    const by = el.getAttribute("aria-labelledby");
    if (by) {
      const t = by.split(/\s+/).map((id) => document.getElementById(id)).filter(Boolean).map((n) => n.textContent).join(" ");
      if (squash(t)) return t;
    }
    return el.getAttribute("aria-label");
  };
  for (const el of document.querySelectorAll('a[href], button, [role=button], [role=link], input[type=submit], input[type=button]')) {
    if (!shown(el) || el.disabled) continue;
    const name = nameFromAttrs(el);
    if (name === null || name === undefined || !squash(name)) continue; // no attribute name: the name is the visible text
    const isInput = el.tagName === "INPUT";
    const visible = isInput ? el.value : el.innerText;
    const v = words(visible);
    // Text that is only a symbol or an icon has no words to compare.
    if (!v || v.length < 2) continue;
    if (!words(name).includes(v)) {
      push(out.labelInName, { selector: selector(el), detail: "it shows \"" + squash(visible).slice(0, 60) + "\" but its name is \"" + squash(name).slice(0, 80) + "\"" });
    }
  }

  const others = /\b(friends?|colleagues?|recipients?|their|someone|invit\w*|referral|referrer|share with|gift|teammates?|members?|admin\w*)\b/i;
  const labelText = (el) => {
    const bits = [el.getAttribute("aria-label")];
    if (el.id) for (const l of document.querySelectorAll("label[for=\"" + CSS.escape(el.id) + "\"]")) bits.push(l.textContent);
    const wrap = el.closest("label");
    if (wrap) bits.push(wrap.textContent);
    bits.push(el.getAttribute("placeholder"));
    return squash(bits.filter(Boolean).join(" "));
  };
  const expected = (el, text) => {
    const t = el.type;
    const id = ((el.name || "") + " " + (el.id || "")).toLowerCase();
    if (t === "email") return "email";
    if (t === "tel") return "tel";
    if (t !== "text" && t !== "") return null;
    // The label text can hold a placeholder and a trailing "*" too, so it is matched on word boundaries.
    if (/(^|[^a-z])(given|first)[-_ ]?name([^a-z]|$)/.test(id) || /\b(given|first) name\b/i.test(text)) return "given-name";
    if (/(^|[^a-z])(family[-_ ]?name|last[-_ ]?name|surname)([^a-z]|$)/.test(id) || /\b(last|family) name\b|\bsurname\b/i.test(text)) return "family-name";
    if (/(^|[^a-z])(post(al)?|zip)[-_ ]?code([^a-z]|$)/.test(id) || /\b(post(al)? ?code|zip code)\b/i.test(text)) return "postal-code";
    return null;
  };
  for (const el of document.querySelectorAll("input")) {
    if (!shown(el) || el.disabled || el.readOnly || ["hidden", "password", "search", "checkbox", "radio", "submit", "button", "file"].includes(el.type)) continue;
    if (el.closest('[role=search], form[action*="search" i]')) continue;
    const text = labelText(el);
    if (others.test(text + " " + (el.name || ""))) continue;
    const want = expected(el, text);
    if (!want) continue;
    const have = (el.getAttribute("autocomplete") || "").trim().toLowerCase();
    if (names(have)) continue;
    push(out.autocomplete, { selector: selector(el), detail: "it asks for " + want.replace("-", " ") + (have ? " and autocomplete is \"" + have + "\"" : " with no autocomplete value"), want });
  }

  // The roles and ARIA attributes in use, so they can be looked up in the screen reader support figures. A role
  // attribute can list fallbacks, and the browser uses the first one it knows, so that one counts.
  const ariaRoles = new Set(("alert alertdialog application article banner blockquote button caption cell checkbox code " +
    "columnheader combobox complementary contentinfo definition deletion dialog document emphasis feed figure form generic " +
    "grid gridcell group heading img insertion link list listbox listitem log main mark marquee math menu menubar menuitem " +
    "menuitemcheckbox menuitemradio meter navigation none note option paragraph presentation progressbar radio radiogroup " +
    "region row rowgroup rowheader scrollbar search searchbox separator slider spinbutton status strong subscript " +
    "superscript switch tab table tablist tabpanel term textbox time timer toolbar tooltip tree treegrid treeitem").split(" "));
  const features = new Map();
  for (const el of document.body ? document.body.querySelectorAll("*") : []) {
    const names = [...el.attributes].map((a) => a.name).filter((n) => n.startsWith("aria-"));
    const role = squash(el.getAttribute("role")).toLowerCase().split(" ").find((r) => ariaRoles.has(r));
    if (role) names.push(role);
    if (!names.length || !shown(el)) continue;
    for (const n of names) {
      const f = features.get(n);
      if (f) f.count++;
      else if (features.size < 100) features.set(n, { name: n, selector: selector(el), count: 1 });
    }
  }
  out.ariaFeatures = [...features.values()];
  return out;
}
