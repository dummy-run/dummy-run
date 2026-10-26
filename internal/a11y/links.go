package a11y

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/links"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

// RuleBrokenLink is a link that answers with an error status or can't be reached.
const RuleBrokenLink = "dummyrun-broken-link"

// linksMaxPerPage is the most links one page has checked. Past it, links_omitted counts the rest.
const linksMaxPerPage = 300

// linksTimeout bounds the link check for one page.
const linksTimeout = 3 * time.Minute

// linksJS lists the page's http and https links, without fragments, each once, in page order, with the href as
// the page wrote it, so a finding can name the element. Called with (max).
const linksJS = `(max) => {
  const seen = new Map();
  let total = 0;
  for (const a of document.querySelectorAll("a[href], area[href]")) {
    let u;
    try {
      u = new URL(a.getAttribute("href"), document.baseURI);
    } catch (e) {
      continue;
    }
    if (u.protocol !== "http:" && u.protocol !== "https:") continue;
    u.hash = "";
    const key = u.href;
    if (seen.has(key)) continue;
    total++;
    if (seen.size < max) seen.set(key, a.getAttribute("href"));
  }
  return { links: [...seen].map(([url, href]) => ({ url, href })), omitted: Math.max(total - seen.size, 0) };
}`

// linksFetchJS checks links from inside the page, with the page's cookies and the preview credentials the browser
// adds, for links on a site the sweep reaches through preview access. HEAD first, then GET when HEAD isn't allowed.
// A redirect isn't followed: it counts as an answer, so the browser never goes on to wherever it points, which could
// be a private address. Called with (urlsJSON). Returns [{url, code, error, timeout}].
const linksFetchJS = `async (urlsJSON) => {
  const urls = JSON.parse(urlsJSON);
  const one = async (u) => {
    for (const method of ["HEAD", "GET"]) {
      const ctl = new AbortController();
      const t = setTimeout(() => ctl.abort(), 15000);
      try {
        const r = await fetch(u, { method, credentials: "include", redirect: "manual", signal: ctl.signal });
        if (r.type === "opaqueredirect") return { url: u, code: 302 };
        if (method === "HEAD" && (r.status === 405 || r.status === 501)) continue;
        return { url: u, code: r.status };
      } catch (e) {
        return { url: u, code: 0, error: String(e && e.message || e).slice(0, 120), timeout: ctl.signal.aborted };
      } finally {
        clearTimeout(t);
      }
    }
    return { url: u, code: 0, error: "no answer" };
  };
  const out = [];
  for (let i = 0; i < urls.length; i += 6) out.push(...(await Promise.all(urls.slice(i, i + 6).map(one))));
  return out;
}`

// linkCheck checks the links on the page, once for each URL across the sweep, and adds the broken ones to advice.
// With preview access on, links on the swept sites are checked from the browser, which holds the credentials, and
// lychee only gets the rest, so no credential ever reaches it or another site.
func linkCheck(ctx context.Context, conn *bidi.Conn, cx string, cfg Config, res *PageResult, path string, rep *Report) {
	var found struct {
		Links []struct {
			URL  string `json:"url"`
			Href string `json:"href"`
		} `json:"links"`
		Omitted int `json:"omitted"`
	}
	if err := conn.CallFunctionJSON(ctx, cx, linksJS, &found, linksMaxPerPage); err != nil {
		res.LinksError = firstLine(err.Error())
		return
	}
	if rep.linkResults == nil {
		rep.linkResults = map[string]links.Result{}
	}
	lctx, cancel := context.WithTimeout(ctx, linksTimeout)
	defer cancel()
	var todo []string
	for _, l := range found.Links {
		if _, done := rep.linkResults[l.URL]; !done {
			todo = append(todo, l.URL)
		}
	}
	seen := map[string]bool{}
	private := func(host string) bool {
		v, ok := seen[host]
		if !ok {
			v = links.PrivateHost(lctx, host)
			seen[host] = v
		}
		return v
	}
	plan := planLinks(cfg.URLs, cfg.Access.Active(), todo, private)
	// With --respect-robots, a link robots.txt disallows isn't fetched. Private addresses are already out of the
	// plan, so no robots.txt is fetched from them either.
	robotsOK := func(us []string) []string {
		var kept []string
		for _, u := range us {
			if ok, _ := rep.robotsAllows(lctx, cfg, u); ok {
				kept = append(kept, u)
			} else {
				plan.skipped = append(plan.skipped, u)
			}
		}
		return kept
	}
	plan.lychee, plan.browser = robotsOK(plan.lychee), robotsOK(plan.browser)
	for _, u := range plan.skipped {
		rep.linkResults[u] = links.Result{URL: u, Outcome: links.Skipped}
	}
	viaLychee, viaBrowser, allowPrivate := plan.lychee, plan.browser, plan.allowPrivate
	if len(viaLychee) > 0 {
		bin, err := links.Find()
		if err != nil {
			res.LinksError = err.Error()
			return
		}
		got, err := links.Check(lctx, bin, viaLychee, telemetry.RequestUserAgent(cfg.Identify), allowPrivate)
		if err != nil {
			res.LinksError = firstLine(err.Error())
			return
		}
		for _, r := range got {
			rep.linkResults[r.URL] = r
		}
	}
	if len(viaBrowser) > 0 {
		arg, _ := json.Marshal(viaBrowser)
		var got []struct {
			URL     string `json:"url"`
			Code    int    `json:"code"`
			Error   string `json:"error"`
			Timeout bool   `json:"timeout"`
		}
		if err := conn.CallFunctionJSON(lctx, cx, linksFetchJS, &got, string(arg)); err != nil {
			res.LinksError = firstLine(err.Error())
			return
		}
		for _, g := range got {
			r := links.Result{URL: g.URL, Code: g.Code, Text: g.Error, Outcome: links.Classify(g.Code, g.Timeout)}
			rep.linkResults[g.URL] = r
		}
	}

	f := Finding{Rule: RuleBrokenLink, Help: "A link that answers with an error or can't be reached",
		HelpURL: "https://www.w3.org/WAI/WCAG22/Understanding/link-purpose-in-context.html", Nodes: []Node{}}
	for _, l := range found.Links {
		r, ok := rep.linkResults[l.URL]
		if !ok {
			continue
		}
		res.LinksChecked++
		switch r.Outcome {
		case links.Refused:
			res.LinksRefused++
		case links.TimedOut:
			res.LinksTimedOut++
		case links.Broken:
			detail := l.URL + ": "
			if r.Code > 0 {
				detail += fmt.Sprintf("HTTP %d", r.Code)
				if r.Text != "" {
					detail += " (" + r.Text + ")"
				}
			} else {
				detail += "no answer"
				if r.Text != "" {
					detail += " (" + r.Text + ")"
				}
			}
			n := Node{Target: "a[href=" + cssString(l.Href) + "]", Detail: detail}
			if e := cfg.Allow.Match(RuleBrokenLink, path, n.Target); e != nil {
				n.Allowed, n.Reason = true, e.Reason
			}
			f.Nodes = append(f.Nodes, n)
		}
	}
	res.LinksOmitted = found.Omitted
	if len(f.Nodes) > 0 {
		res.Advice = append(res.Advice, f)
	}
}

// cssString quotes s as a CSS string. Backslashes and double quotes are escaped, and so is any control character,
// as a hex escape with a space after it. Everything else, & and < included, stays as it is.
func cssString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "\\%x ", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// linkPlan says where each link is checked.
type linkPlan struct {
	lychee, browser, skipped []string
	allowPrivate             bool
}

// planLinks sorts the links to check. The browser can read the answer only for a link on the page's own origin, so
// with preview access on, only links on a swept URL's origin, its scheme and host, go through it. A sweep of local
// or private sites may check links on private addresses. Any other sweep skips them, so a page can't send the
// checker to the machine it runs on or the network around it. The rest go to lychee.
func planLinks(sweptURLs []string, access bool, urls []string, private func(host string) bool) linkPlan {
	swept := map[string]bool{}
	plan := linkPlan{allowPrivate: len(sweptURLs) > 0}
	for _, u := range sweptURLs {
		p, err := url.Parse(u)
		if err != nil {
			plan.allowPrivate = false
			continue
		}
		swept[p.Scheme+"://"+p.Host] = true
		if !private(p.Host) {
			plan.allowPrivate = false
		}
	}
	for _, u := range urls {
		p, err := url.Parse(u)
		switch {
		case err == nil && access && swept[p.Scheme+"://"+p.Host]:
			plan.browser = append(plan.browser, u)
		case err != nil || (!plan.allowPrivate && private(p.Host)):
			plan.skipped = append(plan.skipped, u)
		default:
			plan.lychee = append(plan.lychee, u)
		}
	}
	return plan
}
