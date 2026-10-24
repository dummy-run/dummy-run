// Package tech works out what a site is built with, from what the page itself shows: its scripts, globals, meta
// tags, markup and cookies. The run record keeps the result, and the categories suggest which journeys are worth a
// persona's time: a shop has a basket and a checkout, a site with search has a search to try. No model is involved.
//
// The fingerprints here are our own, written from the public markers each platform documents or serves. You can
// add the webappanalyzer set (GPL-3.0) at run time with DUMMYRUN_WEBAPPANALYZER; it is read from your copy and
// never bundled.
package tech

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// Categories a fingerprint can have. Only some suggest journeys.
const (
	Shop        = "shop"
	CMS         = "cms"
	SiteBuilder = "site-builder"
	Framework   = "framework"
	Search      = "search"
	Booking     = "booking"
	Forms       = "forms"
	Payments    = "payments"
	Consent     = "consent"
	Other       = "other"
)

// Fingerprint is how to recognise one technology. Every list is "any of": one match is enough.
type Fingerprint struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	// Globals are window properties, such as Shopify or Drupal.settings. One that exists is a match.
	Globals []string `json:"globals,omitempty"`
	// Scripts are regular expressions for the src of a script element.
	Scripts []string `json:"scripts,omitempty"`
	// Meta maps a meta element's name to a regular expression for its content, such as generator to WordPress.
	Meta map[string]string `json:"meta,omitempty"`
	// Selectors are CSS selectors. One that finds an element is a match.
	Selectors []string `json:"selectors,omitempty"`
	// HTML are regular expressions for the page's markup, its first 500,000 characters.
	HTML []string `json:"html,omitempty"`
	// Cookies are regular expressions for cookie names the page can read.
	Cookies []string `json:"cookies,omitempty"`
	// DOM are elements that must also have an attribute or text matching a regular expression.
	DOM []DOMCheck `json:"dom,omitempty"`
	// Source is "dummyrun" for our own fingerprints and "webappanalyzer" for ones read from your copy.
	Source string `json:"source,omitempty"`
}

// DOMCheck is an element to find by selector, and what one of them must have. Attributes maps an attribute name to a
// regular expression for its value, and Text is a regular expression for its text. Either can match.
type DOMCheck struct {
	Selector   string            `json:"selector"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Text       string            `json:"text,omitempty"`
}

// Found is one technology on the page, with the first marker that matched.
type Found struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Source   string `json:"source"`
	Evidence string `json:"evidence"`
}

// Own is Dummy Run's own fingerprint set.
var Own = []Fingerprint{
	// Shops.
	{Name: "Shopify", Category: Shop, Globals: []string{"Shopify.shop", "ShopifyAnalytics"}, Scripts: []string{`cdn\.shopify\.com`}},
	{Name: "WooCommerce", Category: Shop, Globals: []string{"woocommerce_params", "wc_add_to_cart_params"}, Selectors: []string{"body.woocommerce", "body.woocommerce-page"}, Scripts: []string{`/plugins/woocommerce/`}},
	{Name: "Magento", Category: Shop, Selectors: []string{`script[type="text/x-magento-init"]`}, Scripts: []string{`/static/version\d+/frontend/`}},
	{Name: "BigCommerce", Category: Shop, Globals: []string{"BCData"}, Scripts: []string{`cdn\d*\.bigcommerce\.com`}},
	{Name: "PrestaShop", Category: Shop, Globals: []string{"prestashop"}, Meta: map[string]string{"generator": `PrestaShop`}},
	{Name: "Squarespace Commerce", Category: Shop, Selectors: []string{`[data-controller="ProductCartButton"]`, ".sqs-add-to-cart-button"}},
	// Content management and site builders.
	{Name: "WordPress", Category: CMS, Meta: map[string]string{"generator": `^WordPress`}, Scripts: []string{`/wp-(content|includes)/`}},
	{Name: "Drupal", Category: CMS, Globals: []string{"Drupal.settings", "drupalSettings"}, Meta: map[string]string{"generator": `^Drupal`}},
	{Name: "Joomla", Category: CMS, Meta: map[string]string{"generator": `Joomla`}},
	{Name: "Ghost", Category: CMS, Meta: map[string]string{"generator": `^Ghost`}},
	{Name: "Squarespace", Category: SiteBuilder, Scripts: []string{`(static1|assets)\.squarespace\.com`}},
	{Name: "Wix", Category: SiteBuilder, Meta: map[string]string{"generator": `Wix\.com`}, Scripts: []string{`static\.parastorage\.com`}},
	{Name: "Webflow", Category: SiteBuilder, Selectors: []string{"html[data-wf-site]"}},
	{Name: "GOV.UK Frontend", Category: Framework, Selectors: []string{".govuk-template", ".govuk-header", ".govuk-template__body"}},
	{Name: "Next.js", Category: Framework, Selectors: []string{"script#__NEXT_DATA__", "#__next"}},
	{Name: "Nuxt", Category: Framework, Globals: []string{"__NUXT__"}, Selectors: []string{"#__nuxt"}},
	{Name: "Angular", Category: Framework, Selectors: []string{"[ng-version]"}},
	// Search.
	{Name: "Site search", Category: Search, Selectors: []string{"form[role=search]", "search form", `input[type=search]`, "[role=searchbox]", `form[action*="search" i] input[type=text]`}},
	{Name: "Algolia", Category: Search, Globals: []string{"algoliasearch"}, Scripts: []string{`algolia(net)?\.(net|com)|algoliasearch`}},
	// Booking.
	{Name: "Calendly", Category: Booking, Selectors: []string{".calendly-inline-widget", `a[href*="calendly.com/"]`}, Scripts: []string{`assets\.calendly\.com`}},
	{Name: "OpenTable", Category: Booking, Scripts: []string{`opentable\.(com|co\.uk)/widget`}},
	{Name: "ResDiary", Category: Booking, Scripts: []string{`resdiary\.com`}},
	{Name: "Acuity Scheduling", Category: Booking, Scripts: []string{`acuityscheduling\.com`}, Selectors: []string{`iframe[src*="acuityscheduling.com"]`}},
	{Name: "SimplyBook.me", Category: Booking, Scripts: []string{`simplybook\.(me|it|asia)`}},
	// Forms.
	{Name: "HubSpot Forms", Category: Forms, Scripts: []string{`js\.hsforms\.net`}, Selectors: []string{".hs-form"}},
	{Name: "Typeform", Category: Forms, Scripts: []string{`embed\.typeform\.com`}, Selectors: []string{"[data-tf-widget]"}},
	{Name: "Google Forms", Category: Forms, Selectors: []string{`iframe[src*="docs.google.com/forms"]`}},
	{Name: "Contact Form 7", Category: Forms, Selectors: []string{".wpcf7"}},
	{Name: "Gravity Forms", Category: Forms, Selectors: []string{".gform_wrapper"}},
	{Name: "Jotform", Category: Forms, Scripts: []string{`jotform\.(com|net)`}, Selectors: []string{`iframe[src*="jotform"]`}},
	// Payments.
	{Name: "Stripe", Category: Payments, Globals: []string{"Stripe"}, Scripts: []string{`js\.stripe\.com`}},
	{Name: "PayPal", Category: Payments, Scripts: []string{`paypal\.com/sdk/js|paypalobjects\.com`}},
	{Name: "Klarna", Category: Payments, Scripts: []string{`klarna(cdn|services)?\.(com|net)`}},
	// Cookie banners.
	{Name: "OneTrust", Category: Consent, Globals: []string{"OneTrust", "Optanon"}, Scripts: []string{`cdn\.cookielaw\.org|optanon`}},
	{Name: "Cookiebot", Category: Consent, Globals: []string{"Cookiebot"}, Scripts: []string{`consent\.cookiebot\.com`}},
	{Name: "Didomi", Category: Consent, Globals: []string{"Didomi"}},
}

// journeys are the journeys a category suggests.
var journeys = map[string][]string{
	Shop:     {"basket", "checkout"},
	Payments: {"checkout"},
	Search:   {"search"},
	Booking:  {"booking"},
	Forms:    {"contact form"},
}

// Journeys returns the journeys the technologies found suggest, in a fixed order, each once.
func Journeys(found []Found) []string {
	order := []string{"search", "basket", "checkout", "booking", "contact form"}
	seen := map[string]bool{}
	for _, f := range found {
		for _, j := range journeys[f.Category] {
			seen[j] = true
		}
	}
	var out []string
	for _, j := range order {
		if seen[j] {
			out = append(out, j)
		}
	}
	return out
}

//go:embed detect.js
var detectJS string

// Detect runs the fingerprints against the loaded page. It runs in the page's own realm, because the globals a
// platform sets are only there, so a page could hide or fake a marker: treat the result as a hint, not proof.
func Detect(ctx context.Context, conn *bidi.Conn, cx string, prints []Fingerprint) ([]Found, error) {
	arg, err := json.Marshal(prints)
	if err != nil {
		return nil, err
	}
	var out []Found
	if err := conn.CallFunctionJSON(ctx, cx, detectJS, &out, string(arg)); err != nil {
		return nil, fmt.Errorf("tech: %w", err)
	}
	// One entry per name: our own fingerprint first, then webappanalyzer's.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Source == "dummyrun" && out[j].Source != "dummyrun" })
	var uniq []Found
	for _, f := range out {
		if !slices.ContainsFunc(uniq, func(u Found) bool { return u.Name == f.Name }) {
			uniq = append(uniq, f)
		}
	}
	sort.SliceStable(uniq, func(i, j int) bool { return uniq[i].Name < uniq[j].Name })
	return uniq, nil
}
