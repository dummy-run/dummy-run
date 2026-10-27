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
	const guard = (root) => {
		if (root && !guarded.has(root)) { guarded.add(root); root.addEventListener("submit", submitted, true); root.addEventListener("click", clicked, true); }
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

// cdpNode is the part of a DevTools DOM node that guardClosedRoots walks.
type cdpNode struct {
	NodeID          int       `json:"nodeId"`
	ShadowRootType  string    `json:"shadowRootType"`
	Children        []cdpNode `json:"children"`
	ShadowRoots     []cdpNode `json:"shadowRoots"`
	ContentDocument *cdpNode  `json:"contentDocument"`
}

// guardClosedRoots hands every closed shadow root in the tab to sameTab's guard. No script can reach a closed root
// the markup declares unless its element's own code asks for it through ElementInternals, so the guard may never see
// it. A click or key press from the keyboard and mouse comparison can still land on a link or button inside one. In
// Chrome, the DevTools protocol can see every root, and Chrome is the browser whose first request from a new tab
// gets past the guards. Firefox has no passthrough, so this does nothing there, and the opened-tab guard stops that
// request instead.
func guardClosedRoots(ctx context.Context, conn *bidi.Conn, cx string) error {
	session, err := conn.CDPSession(ctx, cx)
	if err != nil {
		return nil
	}
	var doc struct {
		Root cdpNode `json:"root"`
	}
	if err := conn.CDP(ctx, session, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, &doc); err != nil {
		return fmt.Errorf("closed roots: %w", err)
	}
	var closed []int
	var walk func(n cdpNode)
	walk = func(n cdpNode) {
		for _, r := range n.ShadowRoots {
			if r.ShadowRootType == "closed" {
				closed = append(closed, r.NodeID)
			}
			walk(r)
		}
		for _, c := range n.Children {
			walk(c)
		}
		if n.ContentDocument != nil {
			walk(*n.ContentDocument)
		}
	}
	walk(doc.Root)
	if len(closed) == 0 {
		return nil
	}
	const group = "dummyrun-closed-roots"
	defer func() {
		_ = conn.CDP(ctx, session, "Runtime.releaseObjectGroup", map[string]any{"objectGroup": group}, nil)
	}()
	for _, id := range closed {
		var res struct {
			Object struct {
				ObjectID string `json:"objectId"`
			} `json:"object"`
		}
		// A root can go between the two calls, when a script removes its element.
		if err := conn.CDP(ctx, session, "DOM.resolveNode", map[string]any{"nodeId": id, "objectGroup": group}, &res); err != nil || res.Object.ObjectID == "" {
			continue
		}
		if err := conn.CDP(ctx, session, "Runtime.callFunctionOn", map[string]any{
			"objectId":            res.Object.ObjectID,
			"functionDeclaration": "function () { const guard = window." + guardRoot + "; if (guard) guard(this); }",
		}, nil); err != nil {
			return fmt.Errorf("closed roots: %w", err)
		}
	}
	return nil
}

// keepToOneTab installs sameTab in every page the browser loads from now on.
func keepToOneTab(ctx context.Context, conn *bidi.Conn) error {
	_, err := conn.AddPreloadScript(ctx, sameTab)
	return err
}
