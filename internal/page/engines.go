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
	//go:embed engines.js
	enginesJS string
)

// EngineVersions are the versions of the bundled engines, by the short name a result uses.
var EngineVersions = map[string]string{
	"ibm":    "accessibility-checker-engine 4.0.34",
	"htmlcs": "HTML_CodeSniffer 2.5.1",
	"alfa":   "Siteimprove Alfa 0.120.0",
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

// EngineGroup is the failures on one element whose WCAG criteria overlap, from one or more engines.
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
}

// RunEngines injects IBM Equal Access, HTML_CodeSniffer and Siteimprove Alfa into a sandbox on the loaded page and
// runs them. An engine that throws is named in Errors and the others still run. Only the criteria in keep are kept.
func RunEngines(ctx context.Context, conn *bidi.Conn, cx string, axe []AxeNode, keep []string) (*EngineResults, error) {
	// Indirect eval runs each bundle at the sandbox's global scope, where HTML_CodeSniffer looks up its standards.
	const load = "(name, src) => { if (!globalThis[name]) (0, eval)(src); return !!globalThis[name]; }"
	for _, b := range []struct{ name, src string }{{"ace", aceJS}, {"HTMLCS", htmlcsJS}, {"dummyrunAlfa", alfaJS}} {
		var loaded bool
		if err := conn.CallFunctionJSONIn(ctx, cx, enginesSandbox, load, &loaded, b.name, b.src); err != nil {
			return nil, fmt.Errorf("engines: load %s: %w", b.name, err)
		}
		if !loaded {
			return nil, fmt.Errorf("engines: %s did not load", b.name)
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
	return &out, nil
}
