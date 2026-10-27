package a11y

import (
	"context"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// sameTab keeps a page from opening a new window. Chrome never reports the first request of a tab a page opens to the
// request interception, so a form sent to a new window would reach the server before any guard saw it. window.open
// does nothing and returns null, and a form, submit button, link or base element that names a new window is sent to
// the page's own tab instead, where the tab's guard decides it. A target that names a frame on the page is left alone.
const sameTab = `() => {
	const frames = (name) => { try { return !!document.querySelector('iframe[name="' + CSS.escape(name) + '"], frame[name="' + CSS.escape(name) + '"]'); } catch (e) { return false; } };
	const away = (t) => { if (!t) return false; const n = String(t).toLowerCase(); return n !== "_self" && n !== "_parent" && n !== "_top" && !frames(t); };
	const keep = (el) => {
		if (!el) return;
		if (away(el.getAttribute && el.getAttribute("target"))) el.setAttribute("target", "_self");
		if (away(el.getAttribute && el.getAttribute("formtarget"))) el.setAttribute("formtarget", "_self");
	};
	const bases = () => document.querySelectorAll("base[target]").forEach(keep);
	window.open = function () { return null; };
	document.addEventListener("submit", (e) => { keep(e.target); keep(e.submitter); bases(); }, true);
	document.addEventListener("click", (e) => { const el = e.target && e.target.closest ? e.target.closest("a, area") : null; keep(el); bases(); }, true);
	const submit = HTMLFormElement.prototype.submit;
	HTMLFormElement.prototype.submit = function () { keep(this); bases(); return submit.call(this); };
	const request = HTMLFormElement.prototype.requestSubmit;
	if (request) HTMLFormElement.prototype.requestSubmit = function (s) { keep(this); keep(s); bases(); return s === undefined ? request.call(this) : request.call(this, s); };
	const click = HTMLElement.prototype.click;
	HTMLElement.prototype.click = function () { if (this.matches && this.matches("a, area")) { keep(this); bases(); } return click.call(this); };
}`

// keepToOneTab installs sameTab in every page the browser loads from now on.
func keepToOneTab(ctx context.Context, conn *bidi.Conn) error {
	_, err := conn.AddPreloadScript(ctx, sameTab)
	return err
}
