// Package price turns token counts into dollars from a checked-in price table. The table is
// prices.json: one entry per model, each with the vendor page it came from and the date it was read.
package price

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed prices.json
var tableJSON []byte

// Entry is the price of one model in US dollars per million tokens.
type Entry struct {
	Model string `json:"model"`
	// Input is the price of an uncached input token.
	Input float64 `json:"input"`
	// CachedInput is the price of an input token served from the prompt cache. Zero means the
	// vendor lists none, and cached tokens cost the input price.
	CachedInput float64 `json:"cached_input"`
	Output      float64 `json:"output"`
	Source      string  `json:"source"`
	Retrieved   string  `json:"retrieved"`
}

// Table is the parsed price table.
type Table struct {
	Version  int     `json:"version"`
	Currency string  `json:"currency"`
	Note     string  `json:"note"`
	Models   []Entry `json:"models"`
}

var table = sync.OnceValues(func() (*Table, error) { return Parse(tableJSON) })

// Parse reads and checks a price table: every entry needs a model, a source, a retrieved date and
// non-negative prices, and a model may appear once.
func Parse(data []byte) (*Table, error) {
	var t Table
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("price: %w", err)
	}
	seen := map[string]bool{}
	for _, e := range t.Models {
		switch {
		case e.Model == "" || e.Source == "" || e.Retrieved == "":
			return nil, fmt.Errorf("price: entry %q needs a model, a source and a retrieved date", e.Model)
		case e.Input < 0 || e.Output < 0 || e.CachedInput < 0:
			return nil, fmt.Errorf("price: entry %q has a negative price", e.Model)
		case seen[e.Model]:
			return nil, fmt.Errorf("price: model %q is listed twice", e.Model)
		}
		seen[e.Model] = true
	}
	return &t, nil
}

// Default returns the embedded table.
func Default() (*Table, error) { return table() }

// Cost returns the cost of a run in dollars, or nil when the model has no entry. inputTokens counts
// every input token, cached ones included, as the run record does.
func (t *Table) Cost(model string, inputTokens, cachedInputTokens, outputTokens int) *float64 {
	for _, e := range t.Models {
		if e.Model != model {
			continue
		}
		cached := min(max(cachedInputTokens, 0), max(inputTokens, 0))
		cachedPrice := e.CachedInput
		if cachedPrice == 0 {
			cachedPrice = e.Input
		}
		usd := (float64(inputTokens-cached)*e.Input + float64(cached)*cachedPrice + float64(max(outputTokens, 0))*e.Output) / 1e6
		return &usd
	}
	return nil
}

// Cost prices a run with the embedded table. It returns nil when the model is unknown.
func Cost(model string, inputTokens, cachedInputTokens, outputTokens int) *float64 {
	t, err := Default()
	if err != nil {
		return nil
	}
	return t.Cost(model, inputTokens, cachedInputTokens, outputTokens)
}
