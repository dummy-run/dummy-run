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
// ElementInternals, the only way a script can, and the sweep never presses a control inside one itself. Values for
// other attributes pass through untouched, so a page that enforces Trusted Types still works. A target that names a
// frame anywhere in the tab is left alone, a sibling of the frame the link or form is in included. A cross-origin
// frame's name can't be read, so a target that names one is sent to the page's own frame.
const sameTab = `() => {
	const named = (w, name) => {
		try { if (w.name === name) return true; } catch (e) {}
		try { for (let i = 0; i < w.frames.length; i++) if (named(w.frames[i], name)) return true; } catch (e) {}
		return false;
	};
	const frames = (name) => named(window.top, name);
	const away = (t) => {
		if (t === null || t === undefined) return false;
		const n = String(t).toLowerCase();
		if (n === "" || n === "_self" || n === "_parent" || n === "_top") return false;
		if (/[\t\n\r]/.test(n) && n.includes("<")) return true;
		return n === "_blank" || !frames(String(t));
	};
	const keep = (el) => {
		if (!el) return;
		if (away(el.getAttribute && el.getAttribute("target"))) el.setAttribute("target", "_self");
		if (away(el.getAttribute && el.getAttribute("formtarget"))) el.setAttribute("formtarget", "_self");
	};
	const bases = () => document.querySelectorAll("base[target]").forEach(keep);
	const listen = EventTarget.prototype.addEventListener, apply = Reflect.apply;
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
