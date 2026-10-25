package page

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// The other rule engines run beside axe-core. Each is a pinned upstream build; see docs/TOOLS.md and NOTICE.
var (
	//go:embed engines/ace.js
	aceJS string
	//go:embed engines/HTMLCS.js
	htmlcsJS string
	//go:embed engines/alfa.min.js
	alfaJS string
	//go:embed engines/qualweb/qw-page.bundle.js
	qwPageJS string
	//go:embed engines/qualweb/util.bundle.js
	qwUtilJS string
	//go:embed engines/qualweb/act.bundle.js
	qwACTJS string
	//go:embed engines/qualweb/wcag.bundle.js
	qwWCAGJS string
	//go:embed engines.js
	enginesJS string
)

// EngineVersions are the versions of the bundled engines, by the short name a result uses.
var EngineVersions = map[string]string{
	"ibm":     "accessibility-checker-engine 4.0.34",
	"htmlcs":  "HTML_CodeSniffer 2.5.1",
	"alfa":    "Siteimprove Alfa 0.120.0",
	"qualweb": "QualWeb act-rules 0.8.6 and wcag-techniques 0.4.9",
}

// enginesSandbox keeps the engines' globals out of the page's own scripts. The DOM is shared.
const enginesSandbox = "dummyrun-engines"

// EnginesMaxGroups is the most failure groups one page returns.
const EnginesMaxGroups = 200

// AxeNode is one element axe-core flagged, with the WCAG criteria of its rule.
type AxeNode struct {
	SC       []string `json:"sc"`
	Selector string   `json:"selector"`
}

// EngineHit is one engine's failed rule on an element.
type EngineHit struct {
	Engine  string `json:"engine"`
	Rule    string `json:"rule"`
	Message string `json:"message,omitempty"`
	Help    string `json:"help,omitempty"`
}

// EngineGroup is the failures on one element for one WCAG criterion, from one or more engines.
type EngineGroup struct {
	Selector string      `json:"selector"`
	SC       []string    `json:"sc"`
	Engines  []string    `json:"engines"`
	Hits     []EngineHit `json:"hits"`
}

// EngineResults are the outcomes of engines.js.
type EngineResults struct {
	Ran    []string          `json:"ran"`
	Errors map[string]string `json:"errors"`
	// Axe holds, for each AxeNode passed in, the other engines that flagged the same element.
	Axe    [][]string    `json:"axe"`
	Groups []EngineGroup `json:"groups"`
	// Omitted counts the groups left out to keep within EnginesMaxGroups. The groups more engines agree on are kept.
	Omitted int `json:"omitted"`
}

// RunEngines injects IBM Equal Access, HTML_CodeSniffer, Siteimprove Alfa and QualWeb into a sandbox on the loaded
// page and runs them. An engine that fails to load or throws is named in Errors and the others still run. Only the criteria in
// keep are kept.
func RunEngines(ctx context.Context, conn *bidi.Conn, cx string, axe []AxeNode, keep []string) (*EngineResults, error) {
	// Indirect eval runs each bundle at the sandbox's global scope, where HTML_CodeSniffer looks up its standards. A
	// bundle that throws or defines nothing is that engine's error, and the others still run. A failed call is fatal,
	// because it means the connection or the page has gone.
	const load = "(name, src) => { try { if (!globalThis[name]) (0, eval)(src); } catch (e) { return String(e && e.message || e).slice(0, 200); } return globalThis[name] ? '' : 'did not load'; }"
	loadErrs := map[string]string{}
	// QualWeb comes as four bundles, loaded in this order: the page model first, then its utilities, then the runners
	// of the ACT rules and the WCAG techniques.
	for _, b := range []struct{ global, engine, src string }{
		{"ace", "ibm", aceJS}, {"HTMLCS", "htmlcs", htmlcsJS}, {"dummyrunAlfa", "alfa", alfaJS},
		{"qwPage", "qualweb", qwPageJS}, {"DomUtils", "qualweb", qwUtilJS},
		{"ACTRulesRunner", "qualweb", qwACTJS}, {"WCAGTechniquesRunner", "qualweb", qwWCAGJS},
	} {
		var msg string
		if err := conn.CallFunctionJSONIn(ctx, cx, enginesSandbox, load, &msg, b.global, b.src); err != nil {
			return nil, fmt.Errorf("engines: load %s: %w", b.engine, err)
		}
		if msg != "" && loadErrs[b.engine] == "" {
			loadErrs[b.engine] = "load: " + msg
		}
	}
	if axe == nil {
		axe = []AxeNode{}
	}
	// Arguments cross BiDi as primitives, so the lists go as JSON text.
	axeArg, _ := json.Marshal(axe)
	keepArg, _ := json.Marshal(keep)
	var out EngineResults
	if err := conn.CallFunctionJSONIn(ctx, cx, enginesSandbox, enginesJS, &out, string(axeArg), string(keepArg), EnginesMaxGroups); err != nil {
		return nil, fmt.Errorf("engines: %w", err)
	}
	for k, v := range loadErrs {
		if out.Errors == nil {
			out.Errors = map[string]string{}
		}
		out.Errors[k] = v
	}
	return &out, nil
}
