package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

func bannerPage(buttons string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body><main><h1>Shop</h1><p>Hello.</p></main>
<div id="banner" role="dialog" aria-label="Cookies" style="position:fixed;bottom:0;left:0;right:0;background:#eee;padding:20px">
<p>We use cookies to improve your experience, for analytics and for advertising.</p>` + buttons + `</div>
<script>document.querySelectorAll("#banner button").forEach(function (b) { b.onclick = function () { document.getElementById("banner").remove(); }; });</script>
</body></html>`
}

func TestConsentRejectsAndReportsAMissingReject(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	pages := map[string]string{
		"/fair":   bannerPage(`<button>Accept all</button><button>Reject all</button>`),
		"/unfair": bannerPage(`<button>Accept all cookies</button><button>Cookie settings</button>`),
		// The banner turns up after the first look, as one loaded by a slow consent script does.
		"/late": strings.Replace(bannerPage(`<button>Accept all</button><button>Reject all</button>`), `id="banner"`, `id="banner" hidden`, 1) +
			`<script>setTimeout(function () { document.getElementById("banner").hidden = false; }, 2000);</script>`,
		// Fourteen other buttons come before the consent ones. autoconsent lists the consent buttons first, so the
		// first 12 still hold the reject button.
		"/many": bannerPage(strings.Repeat(`<button>Learn more</button>`, 14) + `<button>Accept all</button><button>Reject all</button>`),
		"/none": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>None</title></head><body><main><h1>None</h1></main></body></html>`,
	}
	mux := http.NewServeMux()
	for path, body := range pages {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, body)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/fair", srv.URL + "/unfair", srv.URL + "/late", srv.URL + "/many", srv.URL + "/none"}, Browser: browser,
				Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true, Consent: true})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Autoconsent != page.AutoconsentVersion {
				t.Errorf("report autoconsent %q", rep.Autoconsent)
			}
			for _, r := range rep.Results {
				name := strings.TrimPrefix(r.URL, srv.URL)
				if r.Error != "" || r.Consent == nil || r.Consent.Error != "" {
					t.Fatalf("%s: error %q consent %+v", name, r.Error, r.Consent)
				}
				advice := false
				for _, f := range r.Advice {
					if f.Rule == RuleConsentNoReject {
						advice = f.Nodes[0].Target == "#banner"
					}
				}
				switch name {
				case "/many":
					if len(r.Consent.Popups) != 1 || !r.Consent.Popups[0].HasKind("reject") || len(r.Consent.Popups[0].Buttons) > 12 || advice {
						t.Errorf("many buttons: %+v", r.Consent)
					}
				case "/fair", "/late":
					if !r.Consent.OptedOut || !r.Consent.Settled || !r.Consent.Cleared || advice {
						t.Errorf("%s banner: cleared %v, opted out %v, advice %v, %+v", name, r.Consent.Cleared, r.Consent.OptedOut, advice, r.Consent)
					}
				case "/unfair":
					if !advice {
						t.Errorf("unfair banner: no %s advice: %+v", RuleConsentNoReject, r.Consent)
					}
				case "/none":
					if len(r.Consent.Popups) != 0 || advice || !r.Consent.Settled {
						t.Errorf("no banner: %+v", r.Consent)
					}
				}
			}
		})
	}
}

func TestConsentFindingsNeedAcceptWithoutReject(t *testing.T) {
	c := &page.Consent{Popups: []page.ConsentPopup{
		{Selector: "#a", Buttons: []page.ConsentButton{{Text: "Accept", Kind: "accept"}, {Text: "Reject", Kind: "reject"}}},
		{Selector: "#b", Buttons: []page.ConsentButton{{Text: "OK", Kind: "acknowledge"}}},
		{Selector: "#c", Buttons: []page.ConsentButton{{Text: "Accept all", Kind: "accept"}, {Text: "Settings", Kind: "settings"}}},
	}}
	got := consentFindings(c, "/", nil)
	if len(got) != 1 || len(got[0].Nodes) != 1 || got[0].Nodes[0].Target != "#c" ||
		got[0].Nodes[0].Detail != `its buttons are "Accept all" (accept), "Settings" (settings)` {
		t.Errorf("findings: %+v", got)
	}
	if consentFindings(&page.Consent{}, "/", nil) != nil {
		t.Error("no banner should give no finding")
	}
}

func TestConsentThatCouldNotRunFailsThePage(t *testing.T) {
	if msg := consentFailed(&page.Consent{Error: "a step didn't match"}); msg != "" {
		t.Errorf("an error autoconsent reported while it worked failed the page: %q", msg)
	}
	msg := consentFailed(&page.Consent{Failed: true, Error: "boom"})
	if msg != "cookie banner: autoconsent couldn't run: boom" {
		t.Errorf("got %q", msg)
	}
	rep := &Report{Results: []PageResult{{URL: "https://example.com/", Error: msg}}}
	rep.summarise(Config{Allow: &AllowList{}})
	if !rep.Failed() {
		t.Error("a sweep where autoconsent couldn't run passed")
	}
}
