package a11y

import (
	"context"
	"fmt"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// sameTab keeps a page from opening a new window. Chrome never reports the first request of a tab a page opens to the
// request interception, so a form sent to a new window would reach the server before any guard saw it. window.open does
// nothing and returns null, and a form, submit button, link or base element that names a new window is sent to the
// page's own tab instead, where the tab's guard decides it. A target in the markup is changed as the link is clicked or
// the form sent. A script that sets one, through the property, setAttribute or an attribute node, gets _self instead,
// so a page's own click or submit handler can't put _blank back. _blank always means a new window, whatever a window is
// named, and so does a name the browser would treat as _blank. Links and forms inside a shadow root, open or closed,
// are covered too. A closed root declared in the markup is guarded once its element's own code reaches it through
// ElementInternals, the only way a script can, and the sweep never presses a control inside one itself. A closed
// root that setHTMLUnsafe or Document.parseHTMLUnsafe declares after the load is built from markup the guard has
// already put right. The markup is first read into an inert template, where a declared root is still a plain template
// and nothing runs, and the targets in it are changed as they would be on a click. The root stays closed. Markup with
// nothing to change goes through as the page wrote it, and so does a Trusted Types value, which the guard can't
// rewrite. The guard keeps its own copies of what it calls there, so a page that replaces one can't turn it off.
// Values for other attributes pass through untouched, so a page that enforces Trusted Types still works. A target that
// names a frame anywhere in the tab is left alone, a sibling of the frame the link or form is in included. A
// cross-origin frame's name can't be read, so a target that names one is sent to the page's own frame.
const sameTab = `() => {
	const named = (w, name) => {
		try { if (w.name === name) return true; } catch (e) {}
		try { for (let i = 0; i < w.frames.length; i++) if (named(w.frames[i], name)) return true; } catch (e) {}
		return false;
	};
	const frames = (name) => named(window.top, name);
	const apply = Reflect.apply, str = String, exec = RegExp.prototype.exec;
	const lower = String.prototype.toLowerCase, includes = String.prototype.includes;
	const away = (t) => {
		if (t === null || t === undefined) return false;
		const n = apply(lower, str(t), []);
		if (n === "" || n === "_self" || n === "_parent" || n === "_top") return false;
		if (apply(exec, /[\t\n\r]/, [n]) !== null && apply(includes, n, ["<"])) return true;
		return n === "_blank" || !frames(str(t));
	};
	const keep = (el) => {
		if (!el) return;
		if (away(el.getAttribute && el.getAttribute("target"))) el.setAttribute("target", "_self");
		if (away(el.getAttribute && el.getAttribute("formtarget"))) el.setAttribute("formtarget", "_self");
	};
	const bases = () => document.querySelectorAll("base[target]").forEach(keep);
	const listen = EventTarget.prototype.addEventListener;
	window.open = function () { return null; };
	const targets = (name) => { const n = String(name).toLowerCase(); return n === "target" || n === "formtarget"; };
	const same = (name, value) => {
		if (!targets(name)) return value;
		const v = String(value);
		return away(v) ? "_self" : v;
	};
	const property = (proto, name) => {
		const d = proto && Object.getOwnPropertyDescriptor(proto, name);
		if (!d || !d.set) return;
		Object.defineProperty(proto, name, { configurable: true, enumerable: d.enumerable, get: d.get, set: function (v) { const t = String(v); d.set.call(this, away(t) ? "_self" : t); } });
	};
	[HTMLFormElement, HTMLAnchorElement, HTMLAreaElement, HTMLBaseElement].forEach((c) => property(c.prototype, "target"));
	[HTMLButtonElement, HTMLInputElement].forEach((c) => property(c.prototype, "formTarget"));
	const setAttribute = Element.prototype.setAttribute;
	Element.prototype.setAttribute = function (name, value) { return setAttribute.call(this, name, same(name, value)); };
	const setAttributeNS = Element.prototype.setAttributeNS;
	Element.prototype.setAttributeNS = function (ns, name, value) { return setAttributeNS.call(this, ns, name, ns ? value : same(String(name).split(":").pop(), value)); };
	const attr = (a) => { if (a && !a.namespaceURI && targets(a.name) && away(a.value)) a.value = "_self"; return a; };
	["value", "nodeValue", "textContent"].forEach((name) => {
		const proto = name === "value" ? Attr.prototype : Node.prototype;
		const d = Object.getOwnPropertyDescriptor(proto, name);
		if (!d || !d.set) return;
		Object.defineProperty(proto, name, { configurable: true, enumerable: d.enumerable, get: d.get, set: function (v) { d.set.call(this, this instanceof Attr && !this.namespaceURI ? same(this.name, name === "value" || v !== null ? v : "") : v); } });
	});
	[[Element.prototype, "setAttributeNode"], [Element.prototype, "setAttributeNodeNS"], [NamedNodeMap.prototype, "setNamedItem"], [NamedNodeMap.prototype, "setNamedItemNS"]].forEach(([proto, name]) => {
		const f = proto[name];
		if (f) proto[name] = function (a) { return f.call(this, attr(a)); };
	});
	const submitted = (e) => { keep(e.target); keep(e.submitter); bases(); };
	const clicked = (e) => {
		for (const el of e.composedPath ? e.composedPath() : [e.target]) {
			if (!el || !el.matches) continue;
			if (el.matches("a, area")) { keep(el); break; }
			if (el.form && el.matches("button, input")) { keep(el); keep(el.form); break; }
		}
		bases();
	};
	document.addEventListener("submit", submitted, true);
	document.addEventListener("click", clicked, true);
	const guarded = new WeakSet();
	const has = WeakSet.prototype.has.bind(guarded), add = WeakSet.prototype.add.bind(guarded);
	const guard = (root) => {
		if (root && !has(root)) { add(root); apply(listen, root, ["submit", submitted, true]); apply(listen, root, ["click", clicked, true]); }
		return root;
	};
	const attachShadow = Element.prototype.attachShadow;
	if (attachShadow) Element.prototype.attachShadow = function () { return guard(attachShadow.apply(this, arguments)); };
	Object.defineProperty(window, "` + guardRoot + `", { value: guard });
	const internals = typeof ElementInternals !== "undefined" && Object.getOwnPropertyDescriptor(ElementInternals.prototype, "shadowRoot");
	if (internals && internals.get) Object.defineProperty(ElementInternals.prototype, "shadowRoot", { configurable: true, enumerable: internals.enumerable, get: function () { return guard(internals.get.call(this)); } });
	const inner = Object.getOwnPropertyDescriptor(Element.prototype, "innerHTML");
	const outer = Object.getOwnPropertyDescriptor(Element.prototype, "outerHTML").get;
	const getter = (proto, name) => Object.getOwnPropertyDescriptor(proto, name).get;
	const create = Document.prototype.createElement, Parser = DOMParser, parse = DOMParser.prototype.parseFromString;
	const getAttribute = Element.prototype.getAttribute;
	const inFragment = DocumentFragment.prototype.querySelectorAll, inElement = Element.prototype.querySelectorAll;
	const content = getter(HTMLTemplateElement.prototype, "content");
	const top = getter(Document.prototype, "documentElement"), doctype = getter(Document.prototype, "doctype");
	const typeName = getter(DocumentType.prototype, "name"), publicId = getter(DocumentType.prototype, "publicId");
	const systemId = getter(DocumentType.prototype, "systemId");
	const count = getter(NodeList.prototype, "length"), item = NodeList.prototype.item;
	const isHTML = typeof trustedTypes !== "undefined" && trustedTypes.isHTML ? trustedTypes.isHTML.bind(trustedTypes) : () => false;
	const declares = /shadowrootmode/i, sel = "a[target], area[target], form[target], button[formtarget], input[formtarget]";
	const each = (select, root, s, f) => { const list = apply(select, root, [s]); for (let i = 0, n = apply(count, list, []); i < n; i++) f(apply(item, list, [i])); };
	// declared puts right the targets in markup that declares a shadow root. A whole document, for
	// Document.parseHTMLUnsafe, is read with DOMParser, which keeps its doctype and its html, head and body, and
	// builds no root. A part is read into a template. Either way a declared root is still a plain template there and
	// nothing runs. An SVG element named template has no content and declares no root, so it is passed over. If the
	// markup can't be read, the call fails rather than build a root the guard hasn't seen.
	const declared = (s, whole) => {
		if (!inner || apply(exec, declares, [s]) === null) return s;
		let changed = false;
		const visit = (select, root) => {
			each(select, root, sel, (el) => {
				const names = ["target", "formtarget"];
				for (let i = 0; i < 2; i++) {
					if (away(apply(getAttribute, el, [names[i]]))) { apply(setAttribute, el, [names[i], "_self"]); changed = true; }
				}
			});
			each(select, root, "template", (x) => {
				let c;
				try { c = apply(content, x, []); } catch (e) { return; }
				visit(inFragment, c);
			});
		};
		if (whole) {
			const doc = apply(parse, new Parser(), [s, "text/html"]);
			const html = apply(top, doc, []);
			visit(inElement, html);
			if (!changed) return s;
			const dt = apply(doctype, doc, []);
			let head = "";
			if (dt) {
				const pub = apply(publicId, dt, []), sys = apply(systemId, dt, []);
				head = "<!DOCTYPE " + apply(typeName, dt, []) + (pub ? ' PUBLIC "' + pub + '"' + (sys ? ' "' + sys + '"' : "") : sys ? ' SYSTEM "' + sys + '"' : "") + ">";
			}
			return head + apply(outer, html, []);
		}
		const t = apply(create, document, ["template"]);
		apply(inner.set, t, [s]);
		visit(inFragment, apply(content, t, []));
		return changed ? apply(inner.get, t, []) : s;
	};
	// A Trusted Types value goes through as it is. Anything else is read once, and what the browser builds from is
	// the string the guard looked at, so an object whose toString changes between reads can't slip a root past it.
	const parses = (f, self) => function (html, ...rest) {
		if (isHTML(html)) return apply(f, self || this, [html, ...rest]);
		const s = str(html), safe = declared(s, !!self);
		return apply(f, self || this, [safe, ...rest]);
	};
	[Element.prototype, typeof ShadowRoot !== "undefined" && ShadowRoot.prototype].forEach((proto) => {
		if (proto && Object.prototype.hasOwnProperty.call(proto, "setHTMLUnsafe")) proto.setHTMLUnsafe = parses(proto.setHTMLUnsafe);
	});
	if (typeof Document.parseHTMLUnsafe === "function") Document.parseHTMLUnsafe = parses(Document.parseHTMLUnsafe, Document);
	const submit = HTMLFormElement.prototype.submit;
	HTMLFormElement.prototype.submit = function () { keep(this); bases(); return submit.call(this); };
	const request = HTMLFormElement.prototype.requestSubmit;
	if (request) HTMLFormElement.prototype.requestSubmit = function (s) { keep(this); keep(s); bases(); return s === undefined ? request.call(this) : request.call(this, s); };
	const click = HTMLElement.prototype.click;
	HTMLElement.prototype.click = function () { if (this.matches && this.matches("a, area")) { keep(this); bases(); } return click.call(this); };
}`

// guardRoot is the name of the window property, hidden from enumeration, through which guardClosedRoots hands a
// shadow root to sameTab's guard.
const guardRoot = "__dummyrunGuardRoot"

// guardRealm is the sandbox in which guardClosedRoots reads the page. The page's own script can't change what
// document.documentElement returns there, so it can't hide its closed roots behind a decoy.
const guardRealm = "dummyrun-guard"

// domNode is the part of a WebDriver BiDi node remote value that guardClosedRoots walks.
type domNode struct {
	SharedID string `json:"sharedId"`
	Value    struct {
		Mode       string    `json:"mode"`
		Children   []domNode `json:"children"`
		ShadowRoot *domNode  `json:"shadowRoot"`
	} `json:"value"`
}

// guardClosedRoots hands every closed shadow root in the tab and its frames to sameTab's guard. No script can reach
// a closed root the markup declares unless its element's own code asks for it through ElementInternals, so the
// guard may never see it. A click or key press from the keyboard and mouse comparison can still land on a link or
// button inside one, and in Chrome the first request from the tab it opens gets past the request guards. WebDriver
// BiDi can serialise a page with its closed roots, so each one is found and guarded in every browser. A root the
// guard can't take is an error, so the comparison stops rather than press a control that could open a window.
func guardClosedRoots(ctx context.Context, conn *bidi.Conn, cx string) error {
	tree, err := conn.GetTree(ctx)
	if err != nil {
		return fmt.Errorf("closed roots: %w", err)
	}
	var contexts []string
	var frames func(cs []bidi.ContextInfo, in bool)
	frames = func(cs []bidi.ContextInfo, in bool) {
		for _, c := range cs {
			mine := in || c.Context == cx
			if mine {
				contexts = append(contexts, c.Context)
			}
			frames(c.Children, mine)
		}
	}
	frames(tree, false)
	for _, c := range contexts {
		var res struct {
			Type   string  `json:"type"`
			Result domNode `json:"result"`
		}
		if err := conn.Call(ctx, "script.callFunction", map[string]any{
			"functionDeclaration":  "() => document.documentElement",
			"target":               map[string]any{"context": c, "sandbox": guardRealm},
			"awaitPromise":         true,
			"resultOwnership":      "none",
			"serializationOptions": map[string]any{"maxDomDepth": nil, "includeShadowTree": "all"},
		}, &res); err != nil {
			return fmt.Errorf("closed roots: %w", err)
		}
		if res.Type != "success" {
			return fmt.Errorf("closed roots: reading the page failed")
		}
		var closed []string
		var walk func(n *domNode)
		walk = func(n *domNode) {
			if n == nil {
				return
			}
			if r := n.Value.ShadowRoot; r != nil {
				if r.Value.Mode == "closed" {
					closed = append(closed, r.SharedID)
				}
				walk(r)
			}
			for i := range n.Value.Children {
				walk(&n.Value.Children[i])
			}
		}
		walk(&res.Result)
		for _, id := range closed {
			var call struct {
				Type   string `json:"type"`
				Result struct {
					Value bool `json:"value"`
				} `json:"result"`
				ExceptionDetails struct {
					Text string `json:"text"`
				} `json:"exceptionDetails"`
			}
			if err := conn.Call(ctx, "script.callFunction", map[string]any{
				"functionDeclaration": "function () { const guard = window." + guardRoot + "; if (!guard) return false; guard(this); return true; }",
				"target":              map[string]any{"context": c},
				"this":                map[string]any{"sharedId": id},
				"awaitPromise":        true,
				"resultOwnership":     "none",
			}, &call); err != nil {
				return fmt.Errorf("closed roots: %w", err)
			}
			switch {
			case call.Type == "exception":
				return fmt.Errorf("closed roots: the guard threw: %s", call.ExceptionDetails.Text)
			case !call.Result.Value:
				return fmt.Errorf("closed roots: the page has no one-tab guard")
			}
		}
	}
	return nil
}

// keepToOneTab installs sameTab in every page the browser loads from now on.
func keepToOneTab(ctx context.Context, conn *bidi.Conn) error {
	_, err := conn.AddPreloadScript(ctx, sameTab)
	return err
}
