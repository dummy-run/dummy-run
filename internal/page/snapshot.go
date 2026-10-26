package page

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed snapshot.js
var snapshotSrc string

// snapshotJS names elements by the spec computation alone, since it runs in a sandbox of its own with no axe, so a
// snapshot never depends on whether axe loaded.
var snapshotJS = strings.ReplaceAll(withNames(snapshotSrc), "__DR_AXE__", "__dr_snapshot_has_no_axe__")

const snapshotSandbox = "dummyrun-snapshot"

// SnapshotMaxLines is the most lines one ARIA snapshot keeps.
const SnapshotMaxLines = 2000

// ARIASnapshot is an outline of a page's landmarks, headings and controls, one YAML list item a line.
type ARIASnapshot struct {
	Lines []string `json:"lines"`
	// Omitted counts the elements past SnapshotMaxLines.
	Omitted int `json:"omitted"`
}

// TakeARIASnapshot outlines the loaded page.
func TakeARIASnapshot(ctx context.Context, conn *bidi.Conn, cx string) (*ARIASnapshot, error) {
	var out ARIASnapshot
	if err := conn.CallFunctionJSONIn(ctx, cx, snapshotSandbox, snapshotJS, &out, SnapshotMaxLines); err != nil {
		return nil, fmt.Errorf("aria snapshot: %w", err)
	}
	return &out, nil
}

// Text is the snapshot as YAML, ending in a newline.
func (s *ARIASnapshot) Text() string {
	if len(s.Lines) == 0 {
		return ""
	}
	return strings.Join(s.Lines, "\n") + "\n"
}
