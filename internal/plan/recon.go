// Package plan works out what a site is for before any persona visits it, and writes a cast that fits.
//
// It has two stages. Recon reads a few pages over plain HTTP, with no browser and no model: titles,
// headings, menus, forms and signs such as a basket, prices, a sign-in form or a contact link. Plan
// makes one model call over a short summary of that, classifies the site, names its key journeys and
// picks personas from the library with a scenario for each. WriteCast turns the plan into a cast folder
// that "dummyrun run" can play.
package plan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/inject"
	"github.com/dummy-run/dummy-run/internal/robots"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

// MaxPages is how many pages Recon reads.
const MaxPages = 10

// maxPageBytes bounds one page body.
const maxPageBytes = 2 << 20

// Caps on what Recon keeps of each page, so the summary stays small.
const (
	maxHeadings = 8
	maxNav      = 20
	maxForms    = 4
	maxText     = 60
)

// ReconOptions configure Recon.
type ReconOptions struct {
	// Client makes the requests. Nil uses a client with a 20 second timeout.
	Client *http.Client
	// StartURL is the page the scan starts from.
	StartURL string
	// Pages, when given, are the pages to read (the hosted runner passes the sweep's pages). Otherwise
	// Recon reads the sitemap, or failing that the links on the start page.
	Pages []string
	// AllowedHosts are the hosts Recon may fetch. Empty allows the start page's host and its www twin.
	AllowedHosts []string
	// Identify names the scanner page in the user agent.
	Identify bool
	// RespectRobots reads robots.txt and skips the pages it disallows for DummyRun.
	RespectRobots bool
}

// Recon is what the engine learned about a site without a model.
type Recon struct {
	StartURL string `json:"start_url"`
	// Robots says how robots.txt was read, and Disallowed lists the paths it kept from the scan.
	Robots     string   `json:"robots,omitempty"`
	Disallowed []string `json:"disallowed,omitempty"`
	Pages      []Page   `json:"pages"`
	Signals    Signals  `json:"signals"`
	// Skipped counts pages left out for robots.txt, another host or an error.
	Skipped int `json:"skipped"`
}

// Page is one page as Recon read it. Every string here came from the site, so it is data, never instructions.
type Page struct {
	URL      string   `json:"url"`
	Status   int      `json:"status"`
	Lang     string   `json:"lang,omitempty"`
	Title    string   `json:"title,omitempty"`
	Headings []string `json:"headings,omitempty"`
	Nav      []string `json:"nav,omitempty"`
	Forms    []Form   `json:"forms,omitempty"`
	// Text is the start of the visible text, for the summary.
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// Form is one form on a page.
type Form struct {
	Method   string   `json:"method"`
	Inputs   []string `json:"inputs"`
	Password bool     `json:"password,omitempty"`
	Search   bool     `json:"search,omitempty"`
}

// Signals are the signs of what a site is for, counted across the pages Recon read.
type Signals struct {
	SignIn        bool     `json:"sign_in"`
	Basket        bool     `json:"basket"`
	Prices        []string `json:"prices,omitempty"`
	PlansPage     bool     `json:"plans_page"`
	Booking       bool     `json:"booking"`
	Donate        bool     `json:"donate"`
	Subscribe     bool     `json:"subscribe"`
	Contact       []string `json:"contact,omitempty"`
	Articles      bool     `json:"articles"`
	Dates         bool     `json:"dates"`
	Feed          bool     `json:"feed"`
	Docs          bool     `json:"docs"`
	Code          bool     `json:"code"`
	PublicService bool     `json:"public_service"`
	Languages     []string `json:"languages,omitempty"`
}

// Sells is true when something on the site can be bought or paid for.
func (s Signals) Sells() bool { return s.Basket || len(s.Prices) > 0 || s.PlansPage }

var (
	priceRE    = regexp.MustCompile(`[£$€]\s?\d[\d,]*(?:\.\d{2})?`)
	dateRE     = regexp.MustCompile(`(?i)\b(?:\d{1,2}\s+(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+\d{4}|(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+\d{1,2},?\s+\d{4}|\d{4}-\d{2}-\d{2})\b`)
	signInRE   = regexp.MustCompile(`(?i)\b(?:sign|log)\s?-?in\b|\bmy account\b`)
	basketRE   = regexp.MustCompile(`(?i)\b(?:basket|cart|bag|checkout)\b`)
	bookingRE  = regexp.MustCompile(`(?i)\bbook(?:ing)?s?\b(?:\s+(?:a|now|online|table|appointment))?|\breserv(?:e|ation)`)
	donateRE   = regexp.MustCompile(`(?i)\bdonat(?:e|ion)`)
	subRE      = regexp.MustCompile(`(?i)\bsubscribe\b|\bnewsletter\b`)
	plansRE    = regexp.MustCompile(`(?i)\b(?:pricing|plans)\b`)
	contactRE  = regexp.MustCompile(`(?i)\bcontact\b|\bget in touch\b`)
	publicRE   = regexp.MustCompile(`(?i)\bcouncil\b|\bgov\.uk\b|\bnhs\b|\bdepartment for\b`)
	spaceRE    = regexp.MustCompile(`\s+`)
	docsPathRE = regexp.MustCompile(`(?i)(?:^|/)(?:docs?|documentation|guide|guides|reference|manual|api)(?:/|$)`)
)

// Run reads the site. It refuses when robots.txt disallows the start page.
func Run(ctx context.Context, o ReconOptions) (*Recon, error) {
	start, err := url.Parse(o.StartURL)
	if err != nil || start.Host == "" || (start.Scheme != "http" && start.Scheme != "https") {
		return nil, fmt.Errorf("plan: %q is not a web address", o.StartURL)
	}
	allowed := o.AllowedHosts
	if len(allowed) == 0 {
		allowed = twins(start.Hostname())
	}
	// As in the engine's policy, an entry with a port matches that port alone, and a bare host matches the
	// scheme's default port or the start page's own.
	hostOK := func(u *url.URL) bool {
		h, port := strings.ToLower(u.Hostname()), u.Port()
		for _, a := range allowed {
			if strings.EqualFold(a, u.Host) || (strings.EqualFold(a, h) && (port == "" || port == start.Port())) {
				return true
			}
		}
		return false
	}
	ua := telemetry.RequestUserAgent(o.Identify)
	client := guarded(o.Client, ua, hostOK)
	rec := &Recon{StartURL: start.String()}
	rules := robots.AllowAll()
	if o.RespectRobots {
		res, err := robots.Fetch(ctx, client, start.String(), telemetry.RobotsToken, ua)
		if err != nil {
			return nil, err
		}
		rules, rec.Robots = res.Rules, res.Status
		if !rules.Allowed(start.RequestURI()) {
			return rec, ErrRobots
		}
	}
	pages := o.Pages
	if len(pages) == 0 {
		pages = discover(ctx, client, start, hostOK, rules)
	}
	pages = dedupe(append([]string{start.String()}, pages...))
	var all []*pageFacts
	for _, raw := range pages {
		if len(rec.Pages) >= MaxPages {
			break
		}
		u, err := url.Parse(raw)
		if err != nil || !hostOK(u) || (u.Scheme != "http" && u.Scheme != "https") {
			rec.Skipped++
			continue
		}
		if !rules.Allowed(u.RequestURI()) {
			rec.Skipped++
			rec.Disallowed = appendCap(rec.Disallowed, u.Path, 10)
			continue
		}
		p, pf := fetchPage(ctx, client, u)
		if p.Error != "" {
			rec.Skipped++
		}
		rec.Pages = append(rec.Pages, p)
		all = append(all, pf)
	}
	// A scan the caller stopped is not a scan of the site: every page after the stop would say "could not be
	// fetched", and a plan from that would be wrong.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rec.Signals = signals(rec.Pages, all, start)
	return rec, nil
}

// ErrRobots is returned when robots.txt disallows the start page.
var ErrRobots = errors.New("robots.txt disallows DummyRun from the start page")

func twins(host string) []string {
	host = strings.ToLower(host)
	if strings.HasPrefix(host, "www.") {
		return []string{host, strings.TrimPrefix(host, "www.")}
	}
	return []string{host, "www." + host}
}

// guarded copies the client, sets the user agent and refuses a redirect to another host.
func guarded(c *http.Client, ua string, hostOK func(*url.URL) bool) *http.Client {
	if c == nil {
		c = &http.Client{Timeout: 20 * time.Second}
	}
	cp := *c
	next := cp.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	cp.Transport = uaTransport{ua: ua, next: next}
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !hostOK(req.URL) {
			return fmt.Errorf("redirect to %s is outside the site", req.URL.Host)
		}
		if len(via) >= 5 {
			return errors.New("stopped after 5 redirects")
		}
		return nil
	}
	return &cp
}

type uaTransport struct {
	ua   string
	next http.RoundTripper
}

func (t uaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return nil, fmt.Errorf("plan: recon only reads, so %s is refused", r.Method)
	}
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", t.ua)
	return t.next.RoundTrip(r)
}

// discover finds pages: the sitemap named in robots.txt or /sitemap.xml, else the start page's links. It
// spreads the pick across the site's top-level folders.
func discover(ctx context.Context, client *http.Client, start *url.URL, hostOK func(*url.URL) bool, rules *robots.Rules) []string {
	origin := start.Scheme + "://" + start.Host
	var found []string
	allowed := func(_ context.Context, loc string) error {
		u, err := url.Parse(loc)
		if err != nil || !hostOK(u) {
			return errors.New("outside the site")
		}
		return nil
	}
	if locs, err := a11y.FetchSitemap(ctx, client, origin+"/sitemap.xml", allowed); err == nil {
		found = locs
	}
	if len(found) == 0 {
		_, pf := fetchPage(ctx, client, start)
		found = pf.hrefs
	}
	var keep []string
	for _, l := range found {
		u, err := url.Parse(l)
		if err != nil || !hostOK(u) || !rules.Allowed(u.RequestURI()) || skipExt(u.Path) {
			continue
		}
		u.Fragment = ""
		keep = append(keep, u.String())
	}
	return spread(dedupe(keep), MaxPages-1)
}

func skipExt(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case "", ".html", ".htm", ".php", ".aspx", ".asp", ".jsp":
		return false
	}
	return true
}

// spread takes up to n addresses, one per top-level folder in turn, so a blog with a thousand posts does not
// crowd out the about page.
func spread(in []string, n int) []string {
	groups := map[string][]string{}
	var order []string
	for _, raw := range in {
		u, _ := url.Parse(raw)
		key := strings.SplitN(strings.Trim(u.Path, "/"), "/", 2)[0]
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], raw)
	}
	var out []string
	for len(out) < n {
		took := false
		for _, k := range order {
			if len(groups[k]) > 0 && len(out) < n {
				out = append(out, groups[k][0])
				groups[k] = groups[k][1:]
				took = true
			}
		}
		if !took {
			break
		}
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		u, err := url.Parse(s)
		if err != nil {
			continue
		}
		key := strings.ToLower(strings.TrimPrefix(u.Host, "www.")) + strings.TrimSuffix(u.EscapedPath(), "/") + "?" + u.RawQuery
		if !seen[key] {
			seen[key] = true
			out = append(out, s)
		}
	}
	return out
}

func appendCap(l []string, s string, n int) []string {
	if len(l) >= n {
		return l
	}
	return append(l, s)
}

// fetchPage reads one page and returns what it holds, with the facts the summary does not keep as text.
// The facts are never nil.
func fetchPage(ctx context.Context, client *http.Client, u *url.URL) (Page, *pageFacts) {
	p := Page{URL: u.String()}
	none := &pageFacts{}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		p.Error = "bad address"
		return p, none
	}
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		p.Error = "could not be fetched"
		return p, none
	}
	defer func() { _ = resp.Body.Close() }()
	p.Status = resp.StatusCode
	if resp.Request != nil && resp.Request.URL != nil {
		p.URL = resp.Request.URL.String()
	}
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt != "" && mt != "text/html" && mt != "application/xhtml+xml" {
		p.Error = "not a web page (" + mt + ")"
		return p, none
	}
	if resp.StatusCode >= 400 {
		p.Error = fmt.Sprintf("answered %d", resp.StatusCode)
	}
	doc, err := html.Parse(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		p.Error = "could not be read"
		return p, none
	}
	return p, parsePage(doc, resp.Request.URL, &p)
}

// pageFacts holds what parsePage saw that the summary does not keep as text.
type pageFacts struct {
	hrefs    []string
	article  bool
	code     bool
	feed     bool
	contacts []string
	sideNav  bool
	// text is all the visible text, for the signals.
	text string
}

// parsePage fills p from the document and returns what else it saw.
func parsePage(doc *html.Node, base *url.URL, p *Page) *pageFacts {
	pf := &pageFacts{}
	var text strings.Builder
	var walk func(n *html.Node, inNav, inHidden bool)
	walk = func(n *html.Node, inNav, inHidden bool) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Noscript, atom.Template, atom.Svg:
				return
			case atom.Html:
				p.Lang = langTag(attr(n, "lang"))
			case atom.Title:
				if p.Title == "" {
					p.Title = clean(textOf(n), 120)
				}
			case atom.H1, atom.H2, atom.H3:
				if t := clean(textOf(n), 100); t != "" && len(p.Headings) < maxHeadings {
					p.Headings = append(p.Headings, n.Data+" "+t)
				}
			case atom.Nav, atom.Header:
				inNav = true
				if n.DataAtom == atom.Nav && (strings.Contains(strings.ToLower(attr(n, "class")+attr(n, "aria-label")), "side") ||
					strings.Contains(strings.ToLower(attr(n, "class")+attr(n, "aria-label")), "toc")) {
					pf.sideNav = true
				}
			case atom.Aside:
				if strings.Contains(strings.ToLower(attr(n, "class")), "sidebar") {
					pf.sideNav = true
				}
			case atom.Article:
				pf.article = true
			case atom.Pre, atom.Code:
				pf.code = true
			case atom.Link:
				if t := strings.ToLower(attr(n, "type")); strings.Contains(t, "rss") || strings.Contains(t, "atom") {
					pf.feed = true
				}
			case atom.A:
				href := strings.TrimSpace(attr(n, "href"))
				low := strings.ToLower(href)
				switch {
				case strings.HasPrefix(low, "mailto:"):
					pf.contacts = append(pf.contacts, "email link")
				case strings.HasPrefix(low, "tel:"):
					pf.contacts = append(pf.contacts, "phone link")
				case href != "" && !strings.HasPrefix(href, "#"):
					// Only web addresses are pages. That leaves out javascript:, data: and every other scheme.
					if u, err := base.Parse(href); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
						u.Fragment = ""
						pf.hrefs = append(pf.hrefs, u.String())
						if strings.Contains(strings.ToLower(u.Path), "contact") {
							pf.contacts = append(pf.contacts, "contact page")
						}
						if strings.HasSuffix(strings.ToLower(u.Path), ".xml") || strings.Contains(strings.ToLower(u.Path), "/feed") || strings.Contains(strings.ToLower(u.Path), "rss") {
							pf.feed = true
						}
					}
				}
				if inNav && len(p.Nav) < maxNav {
					if t := clean(textOf(n), 40); t != "" {
						p.Nav = append(p.Nav, t)
					}
				}
			case atom.Form:
				if len(p.Forms) < maxForms {
					p.Forms = append(p.Forms, formOf(n))
				}
			}
		}
		if n.Type == html.TextNode && !inHidden {
			if t := strings.TrimSpace(n.Data); t != "" && text.Len() < 4000 {
				text.WriteString(t)
				text.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inNav, inHidden || (c.Type == html.ElementNode && (hasAttr(c, "hidden") || attr(c, "aria-hidden") == "true")))
		}
	}
	walk(doc, false, false)
	full := spaceRE.ReplaceAllString(text.String(), " ")
	p.Text = words(full, maxText)
	pf.contacts = uniq(pf.contacts)
	pf.text = full
	return pf
}

func formOf(n *html.Node) Form {
	f := Form{Method: strings.ToUpper(attr(n, "method"))}
	if f.Method == "" {
		f.Method = "GET"
	}
	if strings.Contains(strings.ToLower(attr(n, "role")+attr(n, "action")+attr(n, "class")), "search") {
		f.Search = true
	}
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		if c.Type == html.ElementNode {
			switch c.DataAtom {
			case atom.Input:
				t := strings.ToLower(attr(c, "type"))
				if t == "" {
					t = "text"
				}
				if t == "hidden" || t == "submit" || t == "button" {
					break
				}
				if t == "password" {
					f.Password = true
				}
				if t == "search" || strings.Contains(strings.ToLower(attr(c, "name")), "search") || attr(c, "name") == "q" {
					f.Search = true
				}
				if len(f.Inputs) < 8 {
					f.Inputs = append(f.Inputs, t)
				}
			case atom.Textarea, atom.Select:
				if len(f.Inputs) < 8 {
					f.Inputs = append(f.Inputs, c.Data)
				}
			}
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			walk(k)
		}
	}
	walk(n)
	return f
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// langRE is the shape of a BCP 47 language tag: a language of two to eight letters and subtags of up to eight
// letters or digits, each after a hyphen.
var langRE = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)

// langTag keeps a lang attribute only when it is shaped like a language tag. Anything else is text the site
// chose, and the summary prints the languages outside the fence, so it is dropped.
func langTag(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 35 || !langRE.MatchString(s) {
		return ""
	}
	return s
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return true
		}
	}
	return false
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		if c.Type == html.ElementNode && (c.DataAtom == atom.Script || c.DataAtom == atom.Style) {
			return
		}
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
			b.WriteByte(' ')
		}
		if c.Type == html.ElementNode && c.DataAtom == atom.Img {
			b.WriteString(attr(c, "alt"))
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			walk(k)
		}
	}
	walk(n)
	return b.String()
}

// clean collapses space, strips invisible characters and caps the length.
func clean(s string, n int) string {
	s = strings.TrimSpace(spaceRE.ReplaceAllString(inject.Strip(s), " "))
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n-1]) + "…"
	}
	return s
}

func words(s string, n int) string {
	f := strings.Fields(inject.Strip(s))
	if len(f) > n {
		f = append(f[:n], "…")
	}
	return strings.Join(f, " ")
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// signals reads the signs of what the site is for from every page.
func signals(pages []Page, all []*pageFacts, start *url.URL) Signals {
	var s Signals
	prices := map[string]bool{}
	langs := map[string]bool{}
	host := strings.ToLower(start.Hostname())
	s.PublicService = strings.HasSuffix(host, ".gov.uk") || strings.HasSuffix(host, ".gov") || strings.HasSuffix(host, ".nhs.uk")
	for i, p := range pages {
		pf := all[i]
		full := pf.text
		if p.Lang != "" {
			langs[strings.ToLower(p.Lang)] = true
		}
		nav := strings.Join(p.Nav, " | ")
		for _, f := range p.Forms {
			if f.Password {
				s.SignIn = true
			}
		}
		if signInRE.MatchString(nav) {
			s.SignIn = true
		}
		if basketRE.MatchString(nav) {
			s.Basket = true
		}
		if plansRE.MatchString(nav) {
			s.PlansPage = true
		}
		if bookingRE.MatchString(nav) {
			s.Booking = true
		}
		if donateRE.MatchString(nav + " " + p.Title) {
			s.Donate = true
		}
		if subRE.MatchString(full) {
			s.Subscribe = true
		}
		if contactRE.MatchString(nav) {
			s.Contact = append(s.Contact, "contact link in the menu")
		}
		for _, m := range priceRE.FindAllString(full, 6) {
			if len(prices) < 6 {
				prices[strings.ReplaceAll(m, " ", "")] = true
			}
		}
		if dateRE.MatchString(full) {
			s.Dates = true
		}
		if publicRE.MatchString(p.Title + " " + nav) {
			s.PublicService = true
		}
		if u, err := url.Parse(p.URL); err == nil && docsPathRE.MatchString(u.Path) {
			s.Docs = true
		}
		s.Articles = s.Articles || pf.article
		s.Code = s.Code || pf.code
		s.Feed = s.Feed || pf.feed
		s.Docs = s.Docs || (pf.sideNav && pf.code)
		s.Contact = append(s.Contact, pf.contacts...)
		for _, h := range pf.hrefs {
			if u, err := url.Parse(h); err == nil {
				lp := strings.TrimSuffix(strings.ToLower(u.Path), "/")
				if strings.Contains(lp, "/cart") || strings.Contains(lp, "/basket") || strings.Contains(lp, "/checkout") {
					s.Basket = true
				}
				if strings.HasSuffix(lp, "/pricing") || strings.HasSuffix(lp, "/plans") {
					s.PlansPage = true
				}
			}
		}
	}
	for p := range prices {
		s.Prices = append(s.Prices, p)
	}
	sort.Strings(s.Prices)
	for l := range langs {
		s.Languages = append(s.Languages, l)
	}
	sort.Strings(s.Languages)
	s.Contact = uniq(s.Contact)
	return s
}

// Summary is the recon as short text for the model. Everything that came from the site is fenced as untrusted
// data with nonce: the pages and the paths robots.txt named. The signals line outside the fence holds only
// yes or no, prices matched by a fixed pattern, fixed contact words and language tags checked for shape.
func (r *Recon) Summary(nonce string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Start page: %s\n", r.StartURL)
	if r.Robots != "" {
		fmt.Fprintf(&b, "robots.txt: %s\n", r.Robots)
	}
	s := r.Signals
	fmt.Fprintf(&b, "Signals the engine measured: sign-in %s, basket or checkout %s, pricing or plans page %s, prices seen %s, booking %s, donate %s, newsletter %s, contact %s, articles %s, dates on pages %s, feed %s, docs %s, code samples %s, public service %s, languages %s.\n",
		yes(s.SignIn), yes(s.Basket), yes(s.PlansPage), list(s.Prices), yes(s.Booking), yes(s.Donate), yes(s.Subscribe), list(s.Contact),
		yes(s.Articles), yes(s.Dates), yes(s.Feed), yes(s.Docs), yes(s.Code), yes(s.PublicService), list(s.Languages))
	var pages strings.Builder
	if len(r.Disallowed) > 0 {
		fmt.Fprintf(&pages, "Paths robots.txt keeps the scan out of: %s\n", strings.Join(r.Disallowed, ", "))
	}
	for _, p := range r.Pages {
		fmt.Fprintf(&pages, "- %s (%d)", p.URL, p.Status)
		if p.Error != "" {
			fmt.Fprintf(&pages, " %s", p.Error)
		}
		pages.WriteByte('\n')
		if p.Title != "" {
			fmt.Fprintf(&pages, "  title: %s\n", p.Title)
		}
		if len(p.Headings) > 0 {
			fmt.Fprintf(&pages, "  headings: %s\n", strings.Join(p.Headings, " | "))
		}
		if len(p.Nav) > 0 {
			fmt.Fprintf(&pages, "  menu: %s\n", strings.Join(p.Nav, " | "))
		}
		for _, f := range p.Forms {
			fmt.Fprintf(&pages, "  form: %s %s%s%s\n", f.Method, strings.Join(f.Inputs, ","), flag(f.Password, " password"), flag(f.Search, " search"))
		}
		if p.Text != "" {
			fmt.Fprintf(&pages, "  text: %s\n", p.Text)
		}
	}
	b.WriteString("Pages read:\n")
	b.WriteString(inject.Fence(nonce, pages.String()))
	return b.String()
}

func yes(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func list(l []string) string {
	if len(l) == 0 {
		return "none"
	}
	return strings.Join(l, ", ")
}

func flag(v bool, s string) string {
	if v {
		return s
	}
	return ""
}
