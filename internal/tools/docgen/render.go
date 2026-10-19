package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/tools/repo"
)

const (
	providersBegin = "<!-- docgen:providers begin -->"
	providersEnd   = "<!-- docgen:providers end -->"
)

func providerNames() []string { return provider.Names }

// replaceBetween swaps the text between the provider table markers for body, keeping the markers.
func replaceBetween(doc, body string) (string, error) {
	begin, end := providersBegin, providersEnd
	b := strings.Index(doc, begin)
	e := strings.Index(doc, end)
	switch {
	case b < 0 || e < 0:
		return "", fmt.Errorf("the markers %s and %s are missing", begin, end)
	case e < b:
		return "", errors.New("the end marker comes before the begin marker")
	}
	return doc[:b+len(begin)] + "\n\n" + strings.TrimRight(body, "\n") + "\n\n" + doc[e:], nil
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func regionText(c provider.Capabilities) string {
	switch {
	case c.RegionSelectable && c.DefaultRegion != "":
		return fmt.Sprintf("`--region` decides the endpoint, `%s` by default", c.DefaultRegion)
	case c.RegionSelectable:
		return "`--region` decides the endpoint"
	case c.DefaultRegion != "":
		return fmt.Sprintf("`%s` unless declared", c.DefaultRegion)
	}
	return "you declare it"
}

// renderProviderTable writes the capability table from provider.CapabilitiesFor.
func renderProviderTable() string {
	var b strings.Builder
	b.WriteString("| Kind | Forced tool | Images in tool results | Reasoning effort | Responses API | Cached tokens reported | Seed sent | Region |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	lacking := map[string][]string{}
	for _, kind := range provider.Kinds() {
		c, _ := provider.CapabilitiesFor(kind)
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s | %s |\n", kind, yesNo(c.ForcedToolChoice),
			yesNo(c.ImagesInToolResults), yesNo(c.ReasoningEffort), yesNo(c.ResponsesAPI), yesNo(c.CachedTokens), yesNo(c.Seed), regionText(c))
		for _, f := range []struct {
			name string
			ok   bool
		}{{"tool calling", c.ToolCalling}, {"images in user messages", c.Images}, {"structured output", c.StructuredOutput}} {
			if !f.ok {
				lacking[f.name] = append(lacking[f.name], "`"+kind+"`")
			}
		}
	}
	b.WriteString("\n")
	if len(lacking) == 0 {
		b.WriteString("Every kind supports tool calling, images in user messages and structured output.\n")
		return b.String()
	}
	for _, name := range []string{"tool calling", "images in user messages", "structured output"} {
		if kinds := lacking[name]; len(kinds) > 0 {
			fmt.Fprintf(&b, "Without %s: %s.\n", name, strings.Join(kinds, ", "))
		}
	}
	return b.String()
}

// helpText runs a command with a help flag and returns what it prints. Help text goes to standard error
// and the exit status is not zero for some commands, so both are read and the status is ignored.
func helpText(dir, bin string, args ...string) (string, error) {
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.CommandContext(context.Background(), bin, args...) //nolint:gosec // a binary this tool just built
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return "", err
	}
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return "", fmt.Errorf("%s %s printed nothing", filepath.Base(bin), strings.Join(args, " "))
	}
	return text, nil
}

type helpSection struct {
	heading string
	intro   string
	bin     string
	dir     string
	args    []string
}

// renderCLI builds docs/CLI.md from the help text of the built commands.
func renderCLI(bins *repo.Binaries, root string) (string, error) {
	exporter := filepath.Join(root, "integrations", "grafana", "exporter")
	sections := []helpSection{
		{"dummyrun", "Every command, and the model providers.", bins.DummyRun, root, []string{"--help"}},
		{"dummyrun run", "Play one persona through one scenario and write a run record.", bins.DummyRun, root, []string{"run", "-h"}},
		{"dummyrun coverage", "Count what a set of run records covers and report it as numbers with their limits. See [COVERAGE.md](COVERAGE.md).", bins.DummyRun, root, []string{"coverage", "-h"}},
		{"dummyrun review", "Confirm or reject the pending findings in run records. Only confirmed findings reach a report. See [ENGINE.md](ENGINE.md).", bins.DummyRun, root, []string{"review", "-h"}},
		{"dummyrun a11y", "Check pages for WCAG 2.2 A and AA failures with no model. `dummyrun audit a11y` is the same command.", bins.DummyRun, root, []string{"a11y", "-h"}},
		{"dummyrun doctor", "List browsers and profile directories that an earlier run left behind, and stop them with `--stop`.", bins.DummyRun, root, []string{"doctor", "-h"}},
		{"dummyrun-otel", "Send run records to an OpenTelemetry backend and Grafana.", bins.OTel, exporter, []string{"--help"}},
		{"dummyrun-otel export", "Send the records in some files or directories once.", bins.OTel, exporter, []string{"export", "-h"}},
		{"dummyrun-otel watch", "Send each new record in a directory as it appears.", bins.OTel, exporter, []string{"watch", "-h"}},
	}
	var b strings.Builder
	b.WriteString("# Command line reference\n\n")
	b.WriteString("<!-- Generated by go generate ./cmd/dummyrun from the commands' own help text. Do not edit. CI fails when this file is out of date. -->\n\n")
	b.WriteString("This page is the output of each command's help, captured from the program. [ENGINE.md](ENGINE.md) explains how to use them.\n")
	for _, s := range sections {
		text, err := helpText(s.dir, s.bin, s.args...)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\n## %s\n\n%s\n\n```text\n%s\n```\n", s.heading, s.intro, text)
	}
	return b.String(), nil
}
