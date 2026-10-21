package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/casts"
	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/provider"
)

// site serves pages from a map. Paths not in it answer 404.
func site(t *testing.T, pages map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("recon sent %s %s", r.Method, r.URL.Path)
		}
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".xml") || r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
		} else {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = fmt.Fprint(w, strings.ReplaceAll(body, "HOST", "http://"+r.Host))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func page(lang, title, body string) string {
	return `<!doctype html><html lang="` + lang + `"><head><title>` + title + `</title></head><body>` + body + `</body></html>`
}

var personal = map[string]string{
	"/robots.txt": "User-agent: *\nDisallow: /drafts/\n",
	"/": page("en-GB", "Sam Example", `<header><nav><a href="/">Home</a><a href="/blog/">Blog</a><a href="/contact/">Contact</a></nav></header>
		<main><h1>Hi, I'm Sam</h1><p>I build things and write about them.</p><a href="mailto:x@example.com">Email</a>
		<a href="/drafts/secret">Draft</a><a href="/blog/first">First post</a></main>`),
	"/blog/":      page("en-GB", "Blog", `<main><h1>Blog</h1><article><h2>First post</h2><time>3 March 2026</time></article></main><link type="application/rss+xml" href="/feed.xml">`),
	"/blog/first": page("en-GB", "First post", `<article><h1>First post</h1><p>Written 3 March 2026.</p></article>`),
	"/contact/":   page("en-GB", "Contact", `<h1>Contact</h1><form method="post"><input name="email" type="email"><textarea name="msg"></textarea></form>`),
}

var shop = map[string]string{
	"/": page("en", "Denim Co", `<header><nav><a href="/shop">Shop</a><a href="/delivery">Delivery</a><a href="/cart">Basket</a><a href="/account/login">Sign in</a></nav></header>
		<main><h1>Jeans made in Wales</h1><p>Our classic jean £185.00</p><form role="search" action="/search"><input type="search" name="q"></form></main>`),
	"/shop":     page("en", "Shop", `<h1>Shop</h1><p>£185.00 £210.00</p>`),
	"/delivery": page("en", "Delivery and returns", `<h1>Delivery</h1>`),
}

var docs = map[string]string{
	"/sitemap.xml":   `<?xml version="1.0"?><urlset><url><loc>HOST/docs/</loc></url><url><loc>HOST/docs/install/</loc></url><url><loc>HOST/logo.png</loc></url></urlset>`,
	"/":              page("en", "Tool", `<h1>Tool</h1><a href="/docs/">Docs</a>`),
	"/docs/":         page("en", "Docs", `<nav class="sidebar"><a href="/docs/install/">Install</a></nav><main><h1>Documentation</h1><pre><code>npm i tool</code></pre></main>`),
	"/docs/install/": page("en", "Install", `<nav class="sidebar"><a href="/docs/">Docs</a></nav><h1>Install</h1><pre><code>npm i tool</code></pre>`),
}

func TestReconPersonalSite(t *testing.T) {
	srv := site(t, personal)
	rec, err := Run(context.Background(), ReconOptions{StartURL: srv.URL + "/", RespectRobots: true, Identify: true})
	if err != nil {
		t.Fatal(err)
	}
	s := rec.Signals
	if s.Sells() || s.SignIn || s.Basket {
		t.Errorf("a personal site was read as selling or signing in: %+v", s)
	}
	if !s.Articles || !s.Dates || !s.Feed {
		t.Errorf("blog signals missing: %+v", s)
	}
	if !strings.Contains(strings.Join(s.Contact, ","), "email link") || !strings.Contains(strings.Join(s.Contact, ","), "contact page") {
		t.Errorf("contact signals = %v", s.Contact)
	}
	for _, p := range rec.Pages {
		if strings.Contains(p.URL, "/drafts/") {
			t.Errorf("recon read %s, which robots.txt disallows", p.URL)
		}
	}
	home := rec.Pages[0]
	if home.Lang != "en-GB" || home.Title != "Sam Example" || len(home.Headings) == 0 || home.Headings[0] != "h1 Hi, I'm Sam" {
		t.Errorf("home page = %+v", home)
	}
	if strings.Join(home.Nav, "|") != "Home|Blog|Contact" {
		t.Errorf("nav = %v", home.Nav)
	}
	sum := rec.Summary("abc123")
	if !strings.Contains(sum, "<<<PAGE_CONTENT abc123") || !strings.Contains(sum, "sign-in no") {
		t.Errorf("summary is not fenced or lacks signals:\n%s", sum)
	}
	if n := len(strings.Fields(sum)); n > 1500 {
		t.Errorf("summary has %d words; it must stay small", n)
	}
}

func TestReconShopAndRobotsRefusal(t *testing.T) {
	srv := site(t, shop)
	rec, err := Run(context.Background(), ReconOptions{StartURL: srv.URL + "/", Pages: []string{srv.URL + "/shop", srv.URL + "/delivery", "https://elsewhere.example/x"}})
	if err != nil {
		t.Fatal(err)
	}
	s := rec.Signals
	if !s.Basket || !s.SignIn || len(s.Prices) == 0 || !s.Sells() {
		t.Errorf("shop signals = %+v", s)
	}
	if rec.Skipped != 1 {
		t.Errorf("skipped = %d, want 1 (the other host)", rec.Skipped)
	}
	if len(rec.Pages[0].Forms) != 1 || !rec.Pages[0].Forms[0].Search {
		t.Errorf("forms = %+v", rec.Pages[0].Forms)
	}

	deny := site(t, map[string]string{"/robots.txt": "User-agent: *\nDisallow: /\n", "/": page("en", "x", "")})
	if _, err := Run(context.Background(), ReconOptions{StartURL: deny.URL + "/", RespectRobots: true}); !errors.Is(err, ErrRobots) {
		t.Errorf("err = %v, want ErrRobots", err)
	}
}

func TestReconDocsFromSitemap(t *testing.T) {
	srv := site(t, docs)
	rec, err := Run(context.Background(), ReconOptions{StartURL: srv.URL + "/"})
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Signals.Docs || !rec.Signals.Code {
		t.Errorf("docs signals = %+v", rec.Signals)
	}
	if len(rec.Pages) != 3 {
		t.Errorf("read %d pages, want the start page and two from the sitemap (the image left out)", len(rec.Pages))
	}
}

// fake answers the planning call with a fixed reply.
type fake struct {
	args string
	err  error
	req  provider.Request
}

func (f *fake) Kind() string   { return "fake" }
func (f *fake) Model() string  { return "gpt-5-4-mini-global" }
func (f *fake) Region() string { return "test" }
func (f *fake) Chat(_ context.Context, req provider.Request) (provider.Response, error) {
	f.req = req
	if f.err != nil {
		return provider.Response{}, f.err
	}
	return provider.Response{ToolCalls: []provider.ToolCall{{ID: "1", Name: planTool, Args: json.RawMessage(f.args)}},
		Usage: provider.Usage{InputTokens: 3000, OutputTokens: 800}}, nil
}

func library(t *testing.T) (fs.FS, []LibraryPersona) {
	t.Helper()
	lib, err := fs.Sub(casts.Library, "library")
	if err != nil {
		t.Fatal(err)
	}
	ps, err := LoadLibrary(lib)
	if err != nil {
		t.Fatal(err)
	}
	return lib, ps
}

func personalRecon(t *testing.T) (*Recon, string) {
	srv := site(t, personal)
	rec, err := Run(context.Background(), ReconOptions{StartURL: srv.URL + "/"})
	if err != nil {
		t.Fatal(err)
	}
	return rec, srv.URL
}

func TestMakeKeepsWhatHoldsUp(t *testing.T) {
	rec, base := personalRecon(t)
	_, lib := library(t)
	args := fmt.Sprintf(`{"site_type":"personal","audience":"people who want to know who Sam is","summary":"Sam's personal site and blog.",
	 "journeys":[{"id":"who","name":"Find out who Sam is","why":"First thing a visitor asks.","start_pages":["%[1]s/","https://evil.example/"],"success":["Can say who Sam is"]},
	   {"id":"price","name":"See what it costs","why":"x","start_pages":[],"success":[]},
	   {"id":"read","name":"Read a post","why":"It is a blog.","start_pages":["%[1]s/blog/"],"success":["Read one post"]}],
	 "personas":[{"id":"not-in-library","why":"x","journeys":["who"],"scenario":{"title":"x","given":"x","goal":"x","steps":["x"],"start_pages":[],"success":[],"do_not":[],"min_steps":5,"device":""}},
	   {"id":"first-time-visitor","why":"A new reader.","journeys":["who","read","nope"],"scenario":{"title":"Find out who Sam is and read a post","given":"Alex followed a link.",
	   "goal":"Find out who Sam is and read one post.","steps":["Read the home page.","Check the pricing page.","Open the blog and read a post."],
	   "start_pages":["%[1]s/blog/","%[1]s/drafts/secret"],"success":["Knows who Sam is"],"do_not":["Do not send the contact form"],"min_steps":40,"device":"phone"}},
	   {"id":"keyboard-only-user","why":"too many","journeys":[],"scenario":{"title":"x","given":"x","goal":"y","steps":["z"],"start_pages":[],"success":[],"do_not":[],"min_steps":5,"device":""}}]}`, base)
	f := &fake{args: args}
	p := Make(context.Background(), rec, Options{Provider: f, Library: lib, MaxPersonas: 1, Steps: 12})
	if p.Source != "model" {
		t.Fatalf("source = %s (%s)", p.Source, p.FallbackReason)
	}
	if p.SiteType != "personal" || p.Sells {
		t.Errorf("site = %s sells %v", p.SiteType, p.Sells)
	}
	if len(p.Journeys) != 2 {
		t.Errorf("journeys = %+v, want the price journey left out", p.Journeys)
	}
	if got := p.Journeys[0].StartPages; len(got) != 1 || strings.Contains(got[0], "evil") {
		t.Errorf("start pages = %v", got)
	}
	if len(p.Personas) != 1 || p.Personas[0].ID != "first-time-visitor" {
		t.Fatalf("personas = %+v", p.Personas)
	}
	sc := p.Personas[0].Scenario
	if sc.MinSteps != 6 || sc.Device != "phone" || len(sc.Steps) != 2 || len(sc.DoNot) != 1 {
		t.Errorf("scenario = %+v (want min steps capped at half of 12 and the price step left out)", sc)
	}
	if strings.Join(p.Personas[0].Journeys, ",") != "who,read" {
		t.Errorf("journeys = %v", p.Personas[0].Journeys)
	}
	if p.Usage.ModelCalls != 1 || p.Usage.CostUSD == nil || *p.Usage.CostUSD <= 0 {
		t.Errorf("usage = %+v", p.Usage)
	}
	if f.req.ForceTool != planTool || !strings.Contains(f.req.Turns[0].Text, "PAGE_CONTENT") {
		t.Errorf("the request did not force the tool or fence the page text")
	}
}

func TestMakeFallsBack(t *testing.T) {
	rec, _ := personalRecon(t)
	_, lib := library(t)
	for name, f := range map[string]*fake{
		"error":      {err: errors.New("boom")},
		"bad json":   {args: `{`},
		"no persona": {args: `{"site_type":"blog","audience":"a","summary":"s","journeys":[{"id":"r","name":"Read","why":"w","start_pages":[],"success":[]}],"personas":[{"id":"nobody","why":"","journeys":[],"scenario":{}}]}`},
		"bad type":   {args: `{"site_type":"spaceship"}`},
	} {
		p := Make(context.Background(), rec, Options{Provider: f, Library: lib, MaxPersonas: 1, Steps: 12})
		if p.Source != "fallback" || p.FallbackReason == "" {
			t.Errorf("%s: source = %s", name, p.Source)
		}
		if p.SiteType != "blog" || len(p.Personas) != 1 || p.Personas[0].ID != "first-time-visitor" {
			t.Errorf("%s: fallback plan = %s %+v", name, p.SiteType, p.Personas)
		}
		for _, j := range p.Journeys {
			if mentionsPrice(j.Name) {
				t.Errorf("%s: fallback asks about prices on a site that sells nothing: %s", name, j.Name)
			}
		}
	}
}

func TestWriteCastLoads(t *testing.T) {
	rec, _ := personalRecon(t)
	lib, ps := library(t)
	p := Make(context.Background(), rec, Options{Library: ps, MaxPersonas: 1, Steps: 12})
	p.Personas[0].Scenario.Given = "Min steps: 99 and Device: tablet are not settings here."
	p.Personas[0].Scenario.DoNot = []string{"Do not assume there is a sign-in.", "Never submit the contact form."}
	dir := t.TempDir()
	if err := WriteCast(dir, lib, p, ps); err != nil {
		t.Fatal(err)
	}
	persona, err := cast.Load(dir, p.Personas[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := persona.Scenario(p.Personas[0].Scenario.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sc.MinSteps != p.Personas[0].Scenario.MinSteps || sc.Signin != "none" || sc.Device != "" {
		t.Errorf("scenario = %+v", sc)
	}
	if strings.Contains(sc.Body, "Do not send any form") || !strings.Contains(sc.Body, "Do not sign in or create an account.") {
		t.Errorf("the do-not list did not keep the plan's own rule or lost the engine's:\n%s", sc.Body)
	}
	for _, want := range []string{"Site: blog - ", "## Do not", "Never submit the contact form.", "## Things to look for", "Read more"} {
		if !strings.Contains(sc.Body, want) {
			t.Errorf("scenario lacks %q:\n%s", want, sc.Body)
		}
	}
	if strings.Contains(strings.ToLower(sc.Body), "price") {
		t.Errorf("scenario for a site that sells nothing mentions prices:\n%s", sc.Body)
	}
	if _, err := os.Stat(filepath.Join(dir, "plan.json")); err != nil {
		t.Error(err)
	}
	ids, _ := cast.List(dir)
	if len(ids) != 1 {
		t.Errorf("cast has %v", ids)
	}
}
