package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/record"
)

func reviewDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	names, err := filepath.Glob(filepath.Join(examples, "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatal("no example records")
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

func runReview(t *testing.T, stdin string, tty bool, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = reviewRun(args, strings.NewReader(stdin), &o, &e, tty)
	return code, o.String(), e.String()
}

func verdicts(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, f := range files {
		rec, err := record.Read(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, fd := range rec.Findings {
			out[rec.ID+"/"+fd.ID] = fd.Verdict
		}
	}
	return out
}

func TestReviewListTableAndJSON(t *testing.T) {
	dir := reviewDir(t)
	code, out, _ := runReview(t, "", false, dir, "--list")
	if code != 0 || !strings.Contains(out, "FINGERPRINT") || !strings.Contains(out, "1718b75300702f7b") || strings.Contains(out, "72be5e1e780d79bb") {
		t.Errorf("exit %d\n%s", code, out)
	}
	code, out, _ = runReview(t, "", false, "--list", "--json", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var items []listItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(items) != 2 || items[0].Fingerprint == "" {
		t.Fatalf("%+v", items)
	}
	for _, it := range items {
		if it.Fingerprint == "1718b75300702f7b" && (it.Pending != 2 || it.Confirmed != 1 || len(it.Occurrences) != 3) {
			t.Errorf("%+v", it)
		}
	}
	code, out, _ = runReview(t, "", false, dir, "--list", "--json", "--verdict", "all")
	if code != 0 || strings.Count(out, `"fingerprint"`) != 5 {
		t.Errorf("exit %d, all problems\n%s", code, out)
	}
	code, out, _ = runReview(t, "", false, dir, "--list", "--verdict", "rejected")
	if code != 0 || !strings.Contains(out, "No rejected findings") {
		t.Errorf("exit %d\n%s", code, out)
	}
	code, out, _ = runReview(t, "", false, dir, "--list", "--json", "--verdict", "rejected")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Errorf("an empty list should be [], got %q", out)
	}
}

func TestReviewConfirmAndRejectWithoutATerminal(t *testing.T) {
	dir := reviewDir(t)
	code, out, errOut := runReview(t, "", false, dir, "--confirm", "87c7d4", "--reject", "1718b75300702f7b", "--reason", "intended wording", "--reviewer", "Pat")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "confirmed 87c7d46615a89adf: 1 finding in 1 run") || !strings.Contains(out, "rejected 1718b75300702f7b: 2 findings in 2 runs") {
		t.Errorf("the output does not say how many findings changed:\n%s", out)
	}
	got := verdicts(t, dir)
	for k, v := range got {
		if v == "pending" {
			t.Errorf("%s is still pending", k)
		}
	}
	if got["2026-10-16-lin-contributor-004/payment-status-wording"] != "rejected" || got["2026-10-16-lin-contributor-002/payment-status-wording"] == "rejected" {
		t.Errorf("the verdict reached the wrong findings: %v", got)
	}
	log := string(mustRead(t, filepath.Join(dir, "review-log.jsonl")))
	if strings.Count(log, "\n") != 2 || !strings.Contains(log, `"reviewer":"Pat"`) || !strings.Contains(log, "intended wording") {
		t.Errorf("log:\n%s", log)
	}
	// Nothing is left, so a second run says so and exits 2.
	if code, _, _ := runReview(t, "", false, dir, "--confirm", "87c7d4", "--reviewer", "Pat"); code != 2 {
		t.Errorf("confirming twice exited %d, want 2", code)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReviewRefusals(t *testing.T) {
	dir := reviewDir(t)
	for name, args := range map[string][]string{
		"no paths":                 {"--list"},
		"reject without reason":    {dir, "--reject", "87c7d4", "--reviewer", "P"},
		"blank reason":             {dir, "--reject", "87c7d4", "--reason", " ", "--reviewer", "P"},
		"reason alone":             {dir, "--reason", "x", "--list"},
		"reason twice":             {dir, "--reject", "87c7d4", "--reason", "a", "--reason", "b", "--reviewer", "P"},
		"list with confirm":        {dir, "--list", "--confirm", "87c7d4"},
		"unknown selector":         {dir, "--confirm", "nonsense", "--reviewer", "P"},
		"short prefix":             {dir, "--confirm", "87c7", "--reviewer", "P"},
		"both verdicts for one":    {dir, "--confirm", "87c7d4", "--reject", "87c7d46615a89adf", "--reason", "x", "--reviewer", "P"},
		"bad verdict filter":       {dir, "--list", "--verdict", "maybe"},
		"missing directory":        {filepath.Join(dir, "none"), "--list"},
		"no terminal and no flags": {dir, "--reviewer", "P"},
		"unknown flag":             {dir, "--bogus"},
	} {
		before := verdicts(t, dir)
		if code, _, _ := runReview(t, "c\n", false, args...); code != 2 {
			t.Errorf("%s: exit %d, want 2", name, code)
		}
		for k, v := range verdicts(t, dir) {
			if before[k] != v {
				t.Errorf("%s: a refused command changed %s", name, k)
			}
		}
	}
}

func TestReviewWritesNothingWhenOneSelectorIsWrong(t *testing.T) {
	dir := reviewDir(t)
	before := verdicts(t, dir)
	if code, _, _ := runReview(t, "", false, dir, "--confirm", "87c7d4", "--confirm", "nonsense", "--reviewer", "P"); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	for k, v := range verdicts(t, dir) {
		if before[k] != v {
			t.Errorf("%s changed", k)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "review-log.jsonl")); err == nil {
		t.Error("a log was written")
	}
}

func TestReviewTakesTheReviewerFromGit(t *testing.T) {
	dir := reviewDir(t)
	// A home with no git identity: the command must ask for --reviewer.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Chdir(t.TempDir())
	if code, _, errOut := runReview(t, "", false, dir, "--confirm", "87c7d4"); code != 2 || !strings.Contains(errOut, "no reviewer") {
		t.Errorf("exit %d: %s", code, errOut)
	}
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = Git Person\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	if code, _, errOut := runReview(t, "", false, dir, "--confirm", "87c7d4"); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	rec, err := record.Read(filepath.Join(dir, "2026-10-15-sam-author-001.json"))
	if err != nil || rec.Findings[0].Reviewer != "Git Person" {
		t.Errorf("%v %+v", err, rec.Findings[0])
	}
}

func TestReviewInteractive(t *testing.T) {
	dir := reviewDir(t)
	code, out, errOut := runReview(t, "c\nr\nbecause\n", true, "--reviewer", "Pat", dir)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "--- 1 of 2 ---") || !strings.Contains(out, "Confirmed 1, rejected 1") {
		t.Errorf("%s", out)
	}
	for k, v := range verdicts(t, dir) {
		if v == "pending" {
			t.Errorf("%s is still pending", k)
		}
	}
}

func TestReviewFlagsMayFollowPaths(t *testing.T) {
	dir := reviewDir(t)
	file := filepath.Join(dir, "2026-10-15-sam-author-001.json")
	if code, out, _ := runReview(t, "", false, file, "--list", "--json"); code != 0 || strings.Count(out, `"fingerprint"`) != 1 {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestReviewSaysWhenItSkipsAVersionOneRecord(t *testing.T) {
	dir := reviewDir(t)
	v1 := `{"schema_version":1,"id":"2026-10-01-old-run","started_at":"2026-10-01T10:00:00Z","ended_at":"2026-10-01T10:01:00Z",
"persona":{"id":"p","role":"r","device":"desktop"},"scenario":{"id":"s","title":"t"},
"target":{"adapter":"url","environment":"preview","base_url":"https://x.example"},
"provider":{"kind":"azure","model":"m","region":"uksouth"},"browser":{"name":"chrome","version":"1"},
"outcome":"completed","steps":[],"findings":[],"usage":{"model_calls":1,"input_tokens":1,"output_tokens":1}}`
	if err := os.WriteFile(filepath.Join(dir, "old.json"), []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runReview(t, "", false, dir, "--list")
	if code != 0 || !strings.Contains(errOut, "schema version 1 records cannot be reviewed") {
		t.Errorf("exit %d: %s", code, errOut)
	}
}

func TestOneLineStripsControlCharacters(t *testing.T) {
	if got := oneLine("a\x1b[2Jb\nc"); strings.ContainsAny(got, "\x1b\n") {
		t.Errorf("%q", got)
	}
}
