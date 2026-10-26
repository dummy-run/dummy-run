package page

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed checks.js
var checksJS string

// ChecksMaxItems is the most nodes one markup check returns.
const ChecksMaxItems = 50

// CheckNode is one element a markup check flags, with a measurement in words.
type CheckNode struct {
	Selector string `json:"selector"`
	Detail   string `json:"detail"`
	Want     string `json:"want,omitempty"`
}

// MarkupChecks are the results of the checks in checks.js.
type MarkupChecks struct {
	LabelInName  []CheckNode `json:"labelInName"`
	Autocomplete []CheckNode `json:"autocomplete"`
}

// RunMarkupChecks runs checks.js on the loaded page.
func RunMarkupChecks(ctx context.Context, conn *bidi.Conn, cx string) (*MarkupChecks, error) {
	var out MarkupChecks
	if err := conn.CallFunctionJSONIn(ctx, cx, "dummyrun-checks", checksJS, &out, ChecksMaxItems); err != nil {
		return nil, fmt.Errorf("markup checks: %w", err)
	}
	return &out, nil
}

//go:embed content.js
var contentJS string

// ContentChecks are the results of the checks in content.js, on the words and images a page's authors wrote.
type ContentChecks struct {
	VagueLink    []CheckNode `json:"vagueLink"`
	URLLink      []CheckNode `json:"urlLink"`
	NewWindow    []CheckNode `json:"newWindow"`
	DocumentLink []CheckNode `json:"documentLink"`
	AltFileName  []CheckNode `json:"altFileName"`
	AltRedundant []CheckNode `json:"altRedundant"`
	AltLong      []CheckNode `json:"altLong"`
	FakeHeading  []CheckNode `json:"fakeHeading"`
	AllCaps      []CheckNode `json:"allCaps"`
	Underline    []CheckNode `json:"underline"`
}

// RunContentChecks runs content.js on the loaded page.
func RunContentChecks(ctx context.Context, conn *bidi.Conn, cx string) (*ContentChecks, error) {
	var out ContentChecks
	if err := conn.CallFunctionJSONIn(ctx, cx, "dummyrun-content", contentJS, &out, ChecksMaxItems); err != nil {
		return nil, fmt.Errorf("content checks: %w", err)
	}
	return &out, nil
}
