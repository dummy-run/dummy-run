package a11y

import (
	"strings"

	"github.com/dummy-run/dummy-run/internal/page"
)

// RuleConsentNoReject is the rule ID of a cookie banner that lets you accept on its first layer but not reject.
const RuleConsentNoReject = "dummyrun-consent-no-reject"

// consentFindings turns a banner with an accept button and no reject button into advice. It is not a WCAG failure,
// so it never fails the sweep. Rejecting through a settings page takes more steps, more reading and more precise
// clicks than accepting, which weighs most on people who tire, use a switch or read slowly.
func consentFindings(c *page.Consent, path string, allow *AllowList) []Finding {
	f := Finding{Rule: RuleConsentNoReject,
		Help:    "The cookie banner lets you accept in one step but has no button to reject",
		HelpURL: "https://ico.org.uk/for-organisations/direct-marketing-and-privacy-and-electronic-communications/guide-to-pecr/cookies-and-similar-technologies/",
		Nodes:   []Node{}}
	for _, p := range c.Popups {
		if !p.HasKind("accept") || p.HasKind("reject") {
			continue
		}
		var labels []string
		for _, b := range p.Buttons {
			labels = append(labels, "\""+b.Text+"\" ("+b.Kind+")")
		}
		n := Node{Target: p.Selector, Detail: "its buttons are " + strings.Join(labels, ", ")}
		if e := allow.Match(RuleConsentNoReject, path, n.Target); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		f.Nodes = append(f.Nodes, n)
	}
	if len(f.Nodes) == 0 {
		return nil
	}
	return []Finding{f}
}

// consentFailed says why the page fails when autoconsent couldn't run at all, so a sweep asked to answer cookie
// banners never passes without having tried. It is empty otherwise.
func consentFailed(c *page.Consent) string {
	if c == nil || !c.Failed {
		return ""
	}
	return firstLine("cookie banner: autoconsent couldn't run: " + c.Error)
}
