package review

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/record"
)

const examples = "../../examples/runs"

// Fingerprints in the example records. 1718 is pending in two runs and confirmed in a third.
const (
	fpShared    = "1718b75300702f7b"
	fpLone      = "87c7d46615a89adf"
	fpConfirmed = "72be5e1e780d79bb"
)

// copyExamples copies the example records into a new directory, so a test can review them.
func copyExamples(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	names, err := filepath.Glob(filepath.Join(examples, "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("no example records: %v", err)
	}
	for _, n := range names {
		data, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(n)), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func load(t *testing.T, dir string) *Set {
	t.Helper()
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

var when = time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)

func TestGroupsDedupeByFingerprint(t *testing.T) {
	s := load(t, copyExamples(t))
	pending := s.Groups(Pending)
	if len(pending) != 2 {
		t.Fatalf("got %d pending problems, want 2", len(pending))
	}
	byFP := map[string]*Group{}
	for _, g := range pending {
		byFP[g.Fingerprint] = g
	}
	if g := byFP[fpShared]; g == nil || len(g.Occurrences) != 3 || len(g.Pending()) != 2 {
		t.Errorf("shared problem: %+v", g)
	}
	if g := byFP[fpLone]; g == nil || len(g.Pending()) != 1 {
		t.Errorf("lone problem: %+v", g)
	}
	if got := len(s.Groups("")); got != 5 {
		t.Errorf("%d problems in all, want 5", got)
	}
}

func TestFind(t *testing.T) {
	s := load(t, copyExamples(t))
	for name, c := range map[string]struct {
		sel     string
		want    string
		wantErr error
	}{
		"full fingerprint":      {fpShared, fpShared, nil},
		"prefix":                {"1718b7", fpShared, nil},
		"upper case prefix":     {"87C7D4", fpLone, nil},
		"prefix too short":      {"1718", "", ErrNoMatch},
		"run and finding":       {"2026-10-16-lin-contributor-004/payment-status-wording", fpShared, nil},
		"unique finding id":     {"payment-status-wording", fpShared, nil},
		"empty":                 {"", "", ErrNoMatch},
		"unknown":               {"nonsense", "", ErrNoMatch},
		"already confirmed":     {fpConfirmed, "", ErrNoMatch},
		"wrong run for finding": {"2026-10-15-sam-author-001/payment-status-wording", "", ErrNoMatch},
	} {
		g, err := s.Find(c.sel)
		if !errors.Is(err, c.wantErr) {
			t.Errorf("%s: error %v, want %v", name, err, c.wantErr)
			continue
		}
		if c.wantErr == nil && g.Fingerprint != c.want {
			t.Errorf("%s: got %s, want %s", name, g.Fingerprint, c.want)
		}
	}
}

func TestFindAmbiguousPrefix(t *testing.T) {
	// Two pending problems whose fingerprints share a prefix.
	s := &Set{groups: []*Group{
		{Fingerprint: "aaaaaa1111111111", Occurrences: []Occurrence{{Finding: record.Finding{Verdict: Pending}, Record: &record.Record{}}}},
		{Fingerprint: "aaaaaa2222222222", Occurrences: []Occurrence{{Finding: record.Finding{Verdict: Pending}, Record: &record.Record{}}}},
	}}
	if _, err := s.Find("aaaaaa"); !errors.Is(err, ErrAmbiguous) {
		t.Errorf("got %v, want ErrAmbiguous", err)
	}
}

func TestApplyReachesEveryPendingFindingWithTheFingerprint(t *testing.T) {
	dir := copyExamples(t)
	s := load(t, dir)
	confirmedFile := filepath.Join(dir, "2026-10-16-lin-contributor-002.json")
	before := readFile(t, confirmedFile)

	g, err := s.Find(fpShared)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := Apply(g, Decision{Verdict: Rejected, Reason: "the label is the product's own term", Reviewer: "Test Reviewer", At: when})
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 {
		t.Fatalf("applied to %d findings, want 2", len(applied))
	}
	if !bytes.Equal(before, readFile(t, confirmedFile)) {
		t.Error("a finding that was already reviewed was rewritten")
	}
	for _, f := range []string{"2026-10-16-lin-contributor-004.json", "2026-10-17-lin-contributor-008.json"} {
		rec, err := record.Read(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("%s is no longer valid: %v", f, err)
		}
		for _, fd := range rec.Findings {
			if fd.Fingerprint != fpShared {
				if fd.Verdict != Confirmed || fd.Reviewer != "example-reviewer" {
					t.Errorf("%s: another finding changed: %+v", f, fd)
				}
				continue
			}
			if fd.Verdict != Rejected || fd.Reviewer != "Test Reviewer" || fd.Reason != "the label is the product's own term" ||
				fd.ReviewedAt == nil || !fd.ReviewedAt.Equal(when) {
				t.Errorf("%s: %+v", f, fd)
			}
		}
	}
	// Nothing is pending for the problem now.
	if _, err := load(t, dir).Find(fpShared); !errors.Is(err, ErrNoMatch) {
		t.Errorf("the problem is still pending: %v", err)
	}
}

func TestApplyKeepsEverythingElseInTheRecord(t *testing.T) {
	dir := copyExamples(t)
	path := filepath.Join(dir, "2026-10-15-sam-author-001.json")
	before := readFile(t, path)
	g, _ := load(t, dir).Find(fpLone)
	if _, err := Apply(g, Decision{Verdict: Confirmed, Reviewer: "R", At: when}); err != nil {
		t.Fatal(err)
	}
	var a, b map[string]any
	if err := json.Unmarshal(before, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(readFile(t, path), &b); err != nil {
		t.Fatal(err)
	}
	f := b["findings"].([]any)[0].(map[string]any)
	for _, k := range []string{"reviewer", "reviewed_at"} {
		if _, ok := f[k]; !ok {
			t.Errorf("%s missing", k)
		}
		delete(f, k)
	}
	if _, ok := f["reason"]; ok {
		t.Error("a confirmation without a reason wrote one")
	}
	f["verdict"] = Pending
	if !reflect.DeepEqual(a, b) {
		t.Error("the record changed outside the review fields")
	}
	// Keys keep their order: schema_version stays first.
	if !strings.HasPrefix(string(readFile(t, path)), "{\n  \"schema_version\": 2,\n  \"id\"") {
		t.Errorf("key order changed:\n%.80s", readFile(t, path))
	}
}

func TestApplyEditsSeverityAndTitle(t *testing.T) {
	dir := copyExamples(t)
	g, _ := load(t, dir).Find(fpLone)
	applied, err := Apply(g, Decision{Verdict: Confirmed, Reviewer: "R", At: when, Severity: "major", Title: "Better title"})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := record.Read(applied[0].File)
	if err != nil {
		t.Fatal(err)
	}
	if f := rec.Findings[0]; f.Severity != "major" || f.Title != "Better title" {
		t.Errorf("%+v", f)
	}
	if applied[0].FromSeverity == "major" || applied[0].FromTitle == "Better title" {
		t.Errorf("the log did not keep the old values: %+v", applied[0])
	}
}

func TestDecisionCheck(t *testing.T) {
	for name, c := range map[string]struct {
		d  Decision
		ok bool
	}{
		"confirm":               {Decision{Verdict: Confirmed, Reviewer: "R"}, true},
		"reject with reason":    {Decision{Verdict: Rejected, Reviewer: "R", Reason: "duplicate of a known bug"}, true},
		"reject without reason": {Decision{Verdict: Rejected, Reviewer: "R"}, false},
		"reject blank reason":   {Decision{Verdict: Rejected, Reviewer: "R", Reason: "  "}, false},
		"pending is not a call": {Decision{Verdict: Pending, Reviewer: "R"}, false},
		"no reviewer":           {Decision{Verdict: Confirmed}, false},
		"bad severity":          {Decision{Verdict: Confirmed, Reviewer: "R", Severity: "huge"}, false},
		"good severity":         {Decision{Verdict: Confirmed, Reviewer: "R", Severity: "blocker"}, true},
		"unknown verdict":       {Decision{Verdict: "maybe", Reviewer: "R"}, false},
		"confirm with a reason": {Decision{Verdict: Confirmed, Reviewer: "R", Reason: "seen it myself"}, true},
		"blank reviewer":        {Decision{Verdict: Confirmed, Reviewer: " "}, false},
	} {
		if err := c.d.Check(); (err == nil) != c.ok {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestApplyRefusesBeforeTouchingAnyFile(t *testing.T) {
	dir := copyExamples(t)
	g, _ := load(t, dir).Find(fpShared)
	if _, err := Apply(g, Decision{Verdict: Rejected, Reviewer: "R"}); err == nil {
		t.Fatal("a rejection without a reason was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, LogName)); err == nil {
		t.Error("a refused decision wrote a log line")
	}
}

func TestLogLinePerDecision(t *testing.T) {
	dir := copyExamples(t)
	s := load(t, dir)
	g1, _ := s.Find(fpShared)
	g2, _ := s.Find(fpLone)
	if _, err := Apply(g1, Decision{Verdict: Rejected, Reason: "noise", Reviewer: "R", At: when}); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(g2, Decision{Verdict: Confirmed, Reviewer: "R", At: when}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(readFile(t, filepath.Join(dir, LogName)))), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d log lines, want 2:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	var e LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Fingerprint != fpShared || e.Verdict != Rejected || e.Reason != "noise" || e.Reviewer != "R" || !e.At.Equal(when) || len(e.Applied) != 2 {
		t.Errorf("%+v", e)
	}
	if strings.Contains(e.Applied[0].File, string(filepath.Separator)) {
		t.Errorf("the log should name the file, not its path: %s", e.Applied[0].File)
	}
	// The log is not mistaken for a record.
	if _, err := Load(dir); err != nil {
		t.Errorf("a directory with a log no longer loads: %v", err)
	}
}

func noTemp(t *testing.T, dir string) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(dir, ".review-*"))
	if len(left) > 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

func TestWriteIsAtomicAndLeavesNoTemporaryFile(t *testing.T) {
	dir := copyExamples(t)
	g, _ := load(t, dir).Find(fpLone)
	path := filepath.Join(dir, "2026-10-15-sam-author-001.json")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(g, Decision{Verdict: Confirmed, Reviewer: "R", At: when}); err != nil {
		t.Fatal(err)
	}
	noTemp(t, dir)
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Errorf("mode changed to %v", info.Mode().Perm())
	}
}

func TestAFailedWriteKeepsTheOldRecord(t *testing.T) {
	t.Run("the file no longer parses", func(t *testing.T) {
		dir := copyExamples(t)
		g, _ := load(t, dir).Find(fpLone)
		path := filepath.Join(dir, "2026-10-15-sam-author-001.json")
		if err := os.WriteFile(path, []byte(`{"schema_version": 2, "findings": [`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Apply(g, Decision{Verdict: Confirmed, Reviewer: "R"}); err == nil {
			t.Fatal("no error")
		}
		if got := string(readFile(t, path)); got != `{"schema_version": 2, "findings": [` {
			t.Errorf("the file was changed: %q", got)
		}
		noTemp(t, dir)
	})
	t.Run("the result would not match the schema", func(t *testing.T) {
		dir := copyExamples(t)
		g, _ := load(t, dir).Find(fpLone)
		path := filepath.Join(dir, "2026-10-15-sam-author-001.json")
		before := readFile(t, path)
		// A reviewer name cannot be empty in the schema, and the check catches it even past Decision.Check.
		if _, err := editFile(path, fpLone, Decision{Verdict: "not-a-verdict", Reviewer: "R", At: when}); err == nil {
			t.Fatal("an invalid verdict was written")
		}
		_ = g
		if !bytes.Equal(before, readFile(t, path)) {
			t.Error("the file was changed")
		}
		noTemp(t, dir)
	})
	t.Run("the directory cannot be written", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("directory permissions do not stop this user")
		}
		dir := copyExamples(t)
		g, _ := load(t, dir).Find(fpLone)
		path := filepath.Join(dir, "2026-10-15-sam-author-001.json")
		before := readFile(t, path)
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		if _, err := Apply(g, Decision{Verdict: Confirmed, Reviewer: "R"}); err == nil {
			t.Fatal("no error")
		}
		if !bytes.Equal(before, readFile(t, path)) {
			t.Error("the file was changed")
		}
		if _, err := os.Stat(filepath.Join(dir, LogName)); err == nil {
			t.Error("a log line was written for a decision that did not happen")
		}
	})
}

func TestApplyKeepsChangesMadeSinceLoad(t *testing.T) {
	dir := copyExamples(t)
	g, _ := load(t, dir).Find(fpLone)
	path := filepath.Join(dir, "2026-10-15-sam-author-001.json")
	data := readFile(t, path)
	// Someone else adds a second reviewer to the finding while this session is open.
	data = bytes.Replace(data, []byte(`"verdict": "pending"`), []byte(`"verdict": "pending",
        "reviewed_by": ["someone-else"]`), 1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(g, Decision{Verdict: Confirmed, Reviewer: "R", At: when}); err != nil {
		t.Fatal(err)
	}
	rec, err := record.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rec.Findings[0].ReviewedBy, []string{"someone-else"}) {
		t.Errorf("the change made after loading was lost: %+v", rec.Findings[0])
	}
}

func TestLoadSkipsVersionOneAndRefusesDuplicates(t *testing.T) {
	dir := copyExamples(t)
	v1 := `{"schema_version":1,"id":"2026-10-01-old-run","started_at":"2026-10-01T10:00:00Z","ended_at":"2026-10-01T10:01:00Z",
"persona":{"id":"p","role":"r","device":"desktop"},"scenario":{"id":"s","title":"t"},
"target":{"adapter":"url","environment":"preview","base_url":"https://x.example"},
"provider":{"kind":"azure","model":"m","region":"uksouth"},"browser":{"name":"chrome","version":"1"},
"outcome":"completed","steps":[],"findings":[],"usage":{"model_calls":1,"input_tokens":1,"output_tokens":1}}`
	if err := os.WriteFile(filepath.Join(dir, "old.json"), []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	s := load(t, dir)
	if len(s.Skipped) != 1 || !strings.Contains(s.Skipped[0], "schema version 1") {
		t.Errorf("skipped: %v", s.Skipped)
	}
	if err := os.WriteFile(filepath.Join(dir, "copy.json"), readFile(t, filepath.Join(dir, "2026-10-15-sam-author-001.json")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("two files for one run were accepted")
	}
	if _, err := Load(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing path was accepted")
	}
}

func TestOrderedRoundTrip(t *testing.T) {
	in := `{"b":1.50,"a":[1,{"z":null,"y":"<tag> & \"q\""}],"c":true,"d":12345678901234567890}`
	o, err := parseOrdered([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out, err := formatOrdered(o)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, out); err != nil {
		t.Fatal(err)
	}
	// Order and the text of numbers survive. The only change is the escaping Go's encoder always applies.
	want := `{"b":1.50,"a":[1,{"z":null,"y":"\u003ctag\u003e \u0026 \"q\""}],"c":true,"d":12345678901234567890}`
	if compact.String() != want {
		t.Errorf("got  %s\nwant %s", compact.String(), want)
	}
	for _, bad := range []string{`[1]`, `{"a":1} x`, `{"a":`, ``} {
		if _, err := parseOrdered([]byte(bad)); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestRenderShowsTheEvidenceAndStripsControlCharacters(t *testing.T) {
	dir := copyExamples(t)
	// Give the run a screenshot, a steps.jsonl line and a hostile title.
	runDir := filepath.Join(dir, "2026-10-16-lin-contributor-004", "screens")
	if err := os.MkdirAll(runDir, 0o750); err != nil {
		t.Fatal(err)
	}
	shot := filepath.Join(runDir, "009.png")
	if err := os.WriteFile(shot, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	steps := `{"step":9,"page":{"title":"Payments","focus":{"role":"button","name":"Pay now"}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "2026-10-16-lin-contributor-004", "steps.jsonl"), []byte(steps), 0o600); err != nil {
		t.Fatal(err)
	}
	g, _ := load(t, dir).Find(fpShared)
	var out bytes.Buffer
	Render(&out, g)
	got := out.String()
	for _, want := range []string{"info", "question", "evidence: inferred", "payment status", fpShared, "lin-contributor", "check-payment",
		"3 findings of this problem", "cited steps", "step 9", "focus: button \"Pay now\"", shot, "route /payments", "unclear-label"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if c := clean("a\x1b[31mred\x07\x00b\ttab\n"); strings.ContainsAny(c, "\x1b\x07\x00") || !strings.Contains(c, "\t") {
		t.Errorf("clean: %q", c)
	}
}

func TestObservation(t *testing.T) {
	one, many, zero := 1, 3, 0
	ok := 200
	for name, c := range map[string]struct {
		s    record.Step
		want string
	}{
		"nothing":  {record.Step{}, "nothing flagged"},
		"status":   {record.Step{Status: &ok}, "status 200"},
		"zero":     {record.Step{ConsoleErrors: &zero, A11yViolations: &zero}, "nothing flagged"},
		"singular": {record.Step{ConsoleErrors: &one}, "1 console error"},
		"plural":   {record.Step{Status: &ok, ConsoleErrors: &many, A11yViolations: &many, Route: "/x"}, "status 200, 3 console errors, 3 accessibility violations, route /x"},
	} {
		if got := observation(c.s); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func newSession(in string) (*Session, *bytes.Buffer) {
	var out bytes.Buffer
	return &Session{In: bufio.NewReader(strings.NewReader(in)), Out: &out, Reviewer: "Sess Reviewer", Now: func() time.Time { return when }}, &out
}

func TestSession(t *testing.T) {
	// Pending problems are listed most severe first: fpLone (minor or worse) then fpShared (info).
	type want struct {
		confirmed, rejected, skipped, findings int
		quit                                   bool
		pendingLeft                            int
	}
	for name, c := range map[string]struct {
		input string
		want  want
	}{
		"confirm both":              {"c\nc\n", want{confirmed: 2, findings: 3, pendingLeft: 0}},
		"reject then confirm":       {"r\nnot a problem\nc\n", want{confirmed: 1, rejected: 1, findings: 3, pendingLeft: 0}},
		"skip both":                 {"s\ns\n", want{skipped: 2, pendingLeft: 2}},
		"quit at once":              {"q\n", want{quit: true, pendingLeft: 2}},
		"end of input is quit":      {"", want{quit: true, pendingLeft: 2}},
		"end of input after a key":  {"c\n", want{confirmed: 1, quit: true, findings: 1, pendingLeft: 1}},
		"empty reason cancels":      {"r\n\ns\ns\n", want{skipped: 2, pendingLeft: 2}},
		"unknown key asks again":    {"x\nc\nq\n", want{confirmed: 1, quit: true, findings: 1, pendingLeft: 1}},
		"edit then confirm":         {"e\nblocker\nBetter title\nc\ns\n", want{confirmed: 1, skipped: 1, findings: 1, pendingLeft: 1}},
		"bad severity changes none": {"e\nhuge\ns\nq\n", want{skipped: 1, quit: true, pendingLeft: 2}},
		"upper case and words":      {"CONFIRM\nSkip\n", want{confirmed: 1, skipped: 1, findings: 1, pendingLeft: 1}},
		"last line has no newline":  {"c\nc", want{confirmed: 2, findings: 3, pendingLeft: 0}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := copyExamples(t)
			sess, out := newSession(c.input)
			sum, err := sess.Run(load(t, dir))
			if err != nil {
				t.Fatal(err)
			}
			got := want{sum.Confirmed, sum.Rejected, sum.Skipped, sum.Findings, sum.Quit, len(load(t, dir).Groups(Pending))}
			if got != c.want {
				t.Errorf("got %+v, want %+v\n%s", got, c.want, out.String())
			}
		})
	}
}

func TestSessionAppliesEditAndReason(t *testing.T) {
	dir := copyExamples(t)
	sess, _ := newSession("e\nblocker\nSharper title\nc\nr\nit is intended\n")
	if _, err := sess.Run(load(t, dir)); err != nil {
		t.Fatal(err)
	}
	rec, err := record.Read(filepath.Join(dir, "2026-10-15-sam-author-001.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := rec.Findings[0]
	if f.Verdict != Confirmed || f.Severity != "blocker" || f.Title != "Sharper title" || f.Reviewer != "Sess Reviewer" {
		t.Errorf("%+v", f)
	}
	rec, _ = record.Read(filepath.Join(dir, "2026-10-16-lin-contributor-004.json"))
	if f := rec.Findings[0]; f.Verdict != Rejected || f.Reason != "it is intended" || f.Severity != "info" {
		t.Errorf("an edit leaked into the next problem: %+v", f)
	}
}

func TestSessionWithNothingPending(t *testing.T) {
	dir := copyExamples(t)
	for _, g := range load(t, dir).Groups(Pending) {
		if _, err := Apply(g, Decision{Verdict: Confirmed, Reviewer: "R"}); err != nil {
			t.Fatal(err)
		}
	}
	sess, out := newSession("c\n")
	sum, err := sess.Run(load(t, dir))
	if err != nil || sum != (Summary{}) || !strings.Contains(out.String(), "No pending findings") {
		t.Errorf("%v %+v %q", err, sum, out.String())
	}
}
