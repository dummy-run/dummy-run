package runner

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
)

type reviewer struct{ req provider.Request }

func (*reviewer) Kind() string   { return "openai-compatible" }
func (*reviewer) Model() string  { return "test" }
func (*reviewer) Region() string { return "local" }
func (r *reviewer) Chat(_ context.Context, req provider.Request) (provider.Response, error) {
	r.req = req
	args := `{"decisions":[{"n":1,"decision":"rewrite","reason":"vague","title":"The three \"Read more\" links on /blog/ have the same name","body":"Screen reader users cannot tell them apart."},
		{"n":2,"decision":"observation","reason":"not for this site"},{"n":3,"decision":"drop","reason":"not shown"},{"n":4,"decision":"keep","reason":"fine"}]}`
	return provider.Response{ToolCalls: []provider.ToolCall{{Name: "review", Args: json.RawMessage(args)}},
		Usage: provider.Usage{InputTokens: 1000, OutputTokens: 100}}, nil
}

func TestSelfCheckRewritesDowngradesAndDrops(t *testing.T) {
	p := &reviewer{}
	r := &run{cfg: Config{Provider: p, Scenario: cast.Scenario{Site: "blog - people who read about code",
		Body: "# Read a post\n\n## Goal\n\n\"Read one post.\"\n\n## Steps\n"}}, steps: evidenceSteps(), log: io.Discard}
	f := &Finish{Findings: []FinishFinding{
		{Title: "Links are unclear", Symptom: "unclear-label", Control: "@4", Steps: []int{1}, Fix: "Name each link."},
		{Title: "No pricing", Symptom: "misleading-copy", Quote: "Posted on 3 May", Steps: []int{2}},
		{Title: "Wrong", Symptom: "misleading-copy", Quote: "Latest posts", Steps: []int{1}},
		{Title: "Dead link", Symptom: "dead-link", Steps: []int{5}},
	}}
	if probs := VerifyEvidence(f, r.steps); len(probs) != 0 {
		t.Fatal(probs)
	}
	got := r.selfCheck(context.Background(), f)
	if len(got) != 4 || got[0].Decision != record.CheckRewrite || got[3].Decision != record.CheckKeep {
		t.Fatalf("%+v", got)
	}
	if len(f.Findings) != 2 || !strings.Contains(f.Findings[0].Title, "three") || f.Findings[0].Fix != "Name each link." ||
		f.Findings[0].Body != "Screen reader users cannot tell them apart." || f.Findings[1].Title != "Dead link" {
		t.Errorf("findings after the check: %+v", f.Findings)
	}
	if len(f.Observations) != 1 || f.Observations[0].Kind != record.ObservationDowngraded || f.Observations[0].Text != "No pricing" {
		t.Errorf("notes after the check: %+v", f.Observations)
	}
	if r.usage.ModelCalls != 1 || r.usage.InputTokens != 1000 {
		t.Errorf("the check's usage is not counted: %+v", r.usage)
	}
	text := p.req.Turns[0].Text
	if !strings.Contains(text, "blog - people who read about code") || !strings.Contains(text, "Read one post.") {
		t.Errorf("the check does not see the site line and goal:\n%s", text)
	}
}

// TestEvidenceContextFindsTheQuoteWhateverItsCase checks the text around the quote is found with the case the
// page used, and that text whose lower-case form is longer in bytes cannot push the window past the end of
// the text. U+023A is two bytes and its lower-case form, U+2C65, is three.
func TestEvidenceContextFindsTheQuoteWhateverItsCase(t *testing.T) {
	s := &Step{Obs: page.Observation{Snapshot: &page.Snapshot{Title: "Blog", Headings: []string{"Latest posts"},
		Text: strings.Repeat("Ⱥ", 400) + " Read MORE about the thing."}}}
	got := evidenceContext(s, "read more")
	if !strings.Contains(got, "Title: Blog") || !strings.Contains(got, "Headings: Latest posts") || !strings.Contains(got, "Read MORE about the thing.") {
		t.Errorf("context lost the quote or the page's title:\n%s", got)
	}
	// Prefix the quote with enough text that the window has to start inside the prefix.
	s.Obs.Snapshot.Text = strings.Repeat("Ⱥ", 2000) + " Read MORE about the thing."
	if got := evidenceContext(s, "Read"); !strings.Contains(got, "Read MORE about the thing.") || !utf8.ValidString(got) {
		t.Errorf("a window into non-ASCII text should still hold the quote and whole runes:\n%s", got)
	}
	if got := evidenceContext(s, "nowhere"); !strings.HasPrefix(got, "Title: Blog") || !strings.Contains(got, "Text: Ⱥ") {
		t.Errorf("a quote that is not on the page should give the start of the text:\n%s", got)
	}
	if got := evidenceContext(&Step{}, "x"); got != "" {
		t.Errorf("a step with no snapshot has no context, got %q", got)
	}
}
