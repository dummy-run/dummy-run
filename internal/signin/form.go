package signin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
)

// formTimeout bounds each wait during the form sign-in: the page load and the success check.
const formTimeout = 20 * time.Second

// submitForm signs in through the login form in a tab of its own. The tab is closed afterwards, so
// the persona's tab starts with no history and no form state. The cookies stay in the browser.
// The engine takes no screenshot here, so the password field is never captured.
func submitForm(ctx context.Context, p *page.Page, t policy.Target, c Creds, g *policy.Gate) error {
	cx, err := openLoginTab(ctx, p, t)
	if err != nil {
		return err
	}
	defer closeTab(p, cx)
	conn := p.Conn()
	s := t.Signin
	for _, f := range []struct {
		name  string
		field policy.Field
		value string
	}{{"username", s.Username, c.Username}, {"password", s.Password, c.Password}} {
		var res string
		if err := conn.CallFunctionJSON(ctx, cx, fillFieldJS, &res, f.field.Selector, f.field.Label, f.value); err != nil {
			return fmt.Errorf("fill the %s field: %w", f.name, err)
		}
		if res != "" {
			return fmt.Errorf("the %s field: %s", f.name, res)
		}
	}
	var pos struct {
		Error string  `json:"error"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
	}
	if err := conn.CallFunctionJSON(ctx, cx, findSubmitJS, &pos, s.Submit.Selector, s.Submit.Label); err != nil {
		return fmt.Errorf("find the submit control: %w", err)
	}
	if pos.Error != "" {
		return fmt.Errorf("the submit control: %s", pos.Error)
	}
	if err := conn.Click(ctx, cx, pos.X, pos.Y); err != nil {
		return fmt.Errorf("click the submit control: %w", err)
	}
	return waitForSuccess(ctx, p, cx, s.Success, g)
}

func openLoginTab(ctx context.Context, p *page.Page, t policy.Target) (string, error) {
	conn := p.Conn()
	cx, err := conn.CreateContext(ctx)
	if err != nil {
		return "", fmt.Errorf("open a tab: %w", err)
	}
	nctx, cancel := context.WithTimeout(ctx, formTimeout)
	defer cancel()
	if _, err := conn.Navigate(nctx, cx, t.LoginURL()); err != nil {
		closeTab(p, cx)
		return "", fmt.Errorf("load the login page %s: %w", t.LoginURL(), err)
	}
	return cx, nil
}

func closeTab(p *page.Page, cx string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = p.Conn().CloseContext(ctx, cx)
}

// waitForSuccess polls until the declared request has been sent and the success check holds, or
// the time runs out. A failed sign-in
// usually leaves the page on the login form, so the wait ends on the timeout, and the error lists
// any request that the policy blocked, which is the usual cause when the target file is wrong.
func waitForSuccess(ctx context.Context, p *page.Page, cx string, ok policy.Success, g *policy.Gate) error {
	var re *regexp.Regexp
	if ok.URLMatches != "" {
		re = regexp.MustCompile(ok.URLMatches)
	}
	conn := p.Conn()
	deadline := time.Now().Add(formTimeout)
	for time.Now().Before(deadline) {
		var st struct {
			URL   string `json:"url"`
			Has   bool   `json:"has"`
			Ready bool   `json:"ready"`
		}
		// A navigation in progress can make the script fail. Try again.
		if err := conn.CallFunctionJSON(ctx, cx, successJS, &st, ok.Selector); err == nil {
			urlOK := re == nil || re.MatchString(st.URL)
			selOK := ok.Selector == "" || st.Has
			// The declared request must have been sent, so a success check that also fits the
			// login page cannot end the wait before the form has been posted.
			if _, _, sent := g.Used(); urlOK && selOK && sent && st.Ready {
				return nil
			}
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	msg := fmt.Sprintf("the success check did not hold within %d seconds", int(formTimeout.Seconds()))
	if blocked := p.DrainProblems(); len(blocked) > 0 {
		msg += "; the policy blocked " + strings.Join(blocked, ", ") + ", so check signin.allow_request in the target file"
	}
	return errors.New(msg)
}

// fillFieldJS finds a field by selector or label and sets its value the way a paste would. It
// returns an empty string on success and a short reason otherwise. The reason never holds the value.
const fillFieldJS = `(selector, label, value) => {
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim().toLowerCase();
  const nameOf = (el) => squash(el.getAttribute("aria-label") || [...(el.labels || [])].map((l) => l.innerText).join(" ") || el.placeholder || "");
  let el = null;
  if (selector) {
    try { el = document.querySelector(selector); } catch (e) { return "bad selector"; }
    if (!el) return "nothing matches the selector " + selector;
  } else {
    const want = squash(label);
    const all = [...document.querySelectorAll("input, textarea")].filter((e) => e.type !== "hidden");
    el = all.find((e) => nameOf(e) === want) || all.find((e) => nameOf(e).includes(want));
    if (!el) return "no field is labelled " + JSON.stringify(label);
  }
  if (el.disabled || el.readOnly) return "the field is disabled";
  el.focus();
  const proto = el.tagName === "TEXTAREA" ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  if (setter) setter.call(el, value); else el.value = value;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
  return "";
}`

// findSubmitJS finds the submit control by selector or visible text and returns where to click.
const findSubmitJS = `(selector, label) => {
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim().toLowerCase();
  let el = null;
  if (selector) {
    try { el = document.querySelector(selector); } catch (e) { return { error: "bad selector" }; }
    if (!el) return { error: "nothing matches the selector " + selector };
  } else {
    const want = squash(label);
    const name = (e) => squash(e.getAttribute("aria-label") || e.innerText || e.value || "");
    const all = [...document.querySelectorAll("button, input[type=submit], input[type=button], [role=button], a[href]")];
    el = all.find((e) => name(e) === want) || all.find((e) => name(e).includes(want));
    if (!el) return { error: "no control is named " + JSON.stringify(label) };
  }
  if (el.disabled) return { error: "the control is disabled" };
  el.scrollIntoView({ block: "center", inline: "nearest" });
  const r = el.getBoundingClientRect();
  if (r.width === 0 || r.height === 0) return { error: "the control is not visible" };
  return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
}`

// successJS reads the page address and whether the success selector matches.
const successJS = `(selector) => {
  let has = false;
  if (selector) { try { has = !!document.querySelector(selector); } catch (e) { has = false; } }
  return { url: location.href, has, ready: document.readyState === "complete" };
}`
