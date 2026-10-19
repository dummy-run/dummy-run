package main

import (
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	text := "# Title\n\n<!-- doctest -->\n```sh\ndummyrun version\n```\n\ntext\n\n<!-- doctest site -->\n\n```bash\ndummyrun a11y http://127.0.0.1:1313/\n```\n\n```sh\ndummyrun run\n```\n"
	got, err := extract("x.md", text)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d blocks, want 2", len(got))
	}
	if got[0].Site || got[0].Code != "dummyrun version" || got[0].Line != 3 {
		t.Errorf("first block wrong: %+v", got[0])
	}
	if !got[1].Site || got[1].Line != 10 {
		t.Errorf("second block wrong: %+v", got[1])
	}
}

func TestExtractRejectsStaleMarkers(t *testing.T) {
	for name, text := range map[string]string{
		"no fence":   "<!-- doctest -->\nsome text\n",
		"at the end": "<!-- doctest -->\n",
		"wrong lang": "<!-- doctest -->\n```json\n{}\n```\n",
		"unclosed":   "<!-- doctest -->\n```sh\ndummyrun version\n",
	} {
		if _, err := extract("x.md", text); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCommandsJoinContinuations(t *testing.T) {
	got := commands("# a comment\n\ndummyrun a11y \\\n  --json - \\\n  http://127.0.0.1:1313/\ndummyrun version\n")
	want := []string{"dummyrun a11y --json - http://127.0.0.1:1313/", "dummyrun version"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckSafe(t *testing.T) {
	ok := []string{
		"dummyrun version",
		"dummyrun --help",
		"dummyrun run -h",
		"dummyrun-otel export --dry-run examples/runs",
		"dummyrun-otel export --dry-run examples/runs | head -n 5",
		"dummyrun coverage --runs examples/runs --build a865ffd",
		"dummyrun coverage --runs examples/runs --build a865ffd --format json | head -n 20",
		"dummyrun a11y --sitemap http://127.0.0.1:1313/sitemap.xml --allow site/a11y-allow.txt http://localhost:1313/404.html",
	}
	for _, c := range ok {
		if err := checkSafe(c); err != nil {
			t.Errorf("%q refused: %v", c, err)
		}
	}
	bad := []string{
		"dummyrun run --cast casts/library --persona x --scenario y --target t --out o --model m",
		"dummyrun-otel export examples/runs",
		"dummyrun-otel watch examples/runs",
		"dummyrun a11y https://example.com/",
		"dummyrun coverage --runs examples/runs --out report.md",
		"dummyrun coverage --runs examples/runs --out=report.md",
		"dummyrun version; rm -f x",
		"dummyrun version && curl http://example.com",
		"dummyrun version > out.txt",
		"dummyrun version $(id)",
		"curl http://127.0.0.1:1313/",
		"go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest",
		"dummyrun",
		"dummyrun version | sh",
	}
	for _, c := range bad {
		if err := checkSafe(c); err == nil {
			t.Errorf("%q allowed", c)
		}
	}
}
