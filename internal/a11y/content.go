package a11y

import (
	"github.com/dummy-run/dummy-run/internal/page"
)

// Rule IDs of the content checks, on the words and images a page's authors wrote. Each one is a pattern that
// usually makes a page harder to use, but can be right in context, so all of them are advice and never fail the
// sweep.
const (
	RuleVagueLink    = "dummyrun-content-vague-link"
	RuleURLLink      = "dummyrun-content-url-link"
	RuleNewWindow    = "dummyrun-content-new-window"
	RuleDocumentLink = "dummyrun-content-document-link"
	RuleAltFileName  = "dummyrun-content-alt-file-name"
	RuleAltRedundant = "dummyrun-content-alt-redundant"
	RuleAltLong      = "dummyrun-content-alt-long"
	RuleFakeHeading  = "dummyrun-content-fake-heading"
	RuleAllCaps      = "dummyrun-content-all-caps"
	RuleUnderline    = "dummyrun-content-underline"
)

// ContentRules are the content checks in the order the report lists them.
var ContentRules = []string{RuleAltFileName, RuleVagueLink, RuleFakeHeading, RuleDocumentLink, RuleNewWindow,
	RuleURLLink, RuleAltRedundant, RuleAltLong, RuleAllCaps, RuleUnderline}

var contentRules = map[string]struct{ help, helpURL string }{
	RuleVagueLink: {"A link whose name, such as \"read more\", says nothing about where it goes on its own",
		"https://www.w3.org/WAI/WCAG22/Understanding/link-purpose-in-context.html"},
	RuleURLLink: {"A link whose name is a long web address, which a screen reader reads out character by character",
		"https://www.w3.org/WAI/WCAG22/Understanding/link-purpose-in-context.html"},
	RuleNewWindow: {"A link that opens a new tab or window without saying so",
		"https://www.w3.org/WAI/WCAG22/Understanding/change-on-request.html"},
	RuleDocumentLink: {"A link to a PDF or office document that doesn't say what kind of file it is",
		"https://www.gov.uk/guidance/content-design/links"},
	RuleAltFileName: {"An image whose alt text is a file name, which tells a screen reader user nothing",
		"https://www.w3.org/WAI/WCAG22/Understanding/non-text-content.html"},
	RuleAltRedundant: {"Alt text that starts by saying it is an image, which a screen reader says already",
		"https://www.w3.org/WAI/tutorials/images/tips/"},
	RuleAltLong: {"Alt text of more than 150 characters, which is hard to take in and can't be skimmed",
		"https://www.w3.org/WAI/tutorials/images/complex/"},
	RuleFakeHeading: {"Bold text that looks like a heading but isn't one, so people who move by headings miss it",
		"https://www.w3.org/WAI/WCAG22/Understanding/info-and-relationships.html"},
	RuleAllCaps: {"Text in capitals, which is harder to read and some screen readers spell out",
		"https://guidance.publishing.service.gov.uk/writing-to-gov-uk-standards/style-guides/a-to-z-style-guide/#capitalisation"},
	RuleUnderline: {"Underlined text that isn't a link, so readers may try to follow it",
		"https://www.w3.org/WAI/WCAG22/Understanding/use-of-color.html"},
}

// IsContentRule reports whether the rule ID is one of the content checks.
func IsContentRule(id string) bool { _, ok := contentRules[id]; return ok }

// contentFindings turns the content checks' results into advice, one finding per rule.
func contentFindings(c *page.ContentChecks, path string, allow *AllowList) []Finding {
	by := map[string][]page.CheckNode{
		RuleVagueLink: c.VagueLink, RuleURLLink: c.URLLink, RuleNewWindow: c.NewWindow, RuleDocumentLink: c.DocumentLink,
		RuleAltFileName: c.AltFileName, RuleAltRedundant: c.AltRedundant, RuleAltLong: c.AltLong,
		RuleFakeHeading: c.FakeHeading, RuleAllCaps: c.AllCaps, RuleUnderline: c.Underline,
	}
	var list []Finding
	for _, id := range ContentRules {
		if len(by[id]) == 0 {
			continue
		}
		r := contentRules[id]
		f := Finding{Rule: id, Help: r.help, HelpURL: r.helpURL, Nodes: []Node{}}
		for _, c := range by[id] {
			n := Node{Target: c.Selector, Detail: c.Detail}
			if e := allow.Match(id, path, n.Target); e != nil {
				n.Allowed, n.Reason = true, e.Reason
			}
			f.Nodes = append(f.Nodes, n)
		}
		list = append(list, f)
	}
	return list
}
