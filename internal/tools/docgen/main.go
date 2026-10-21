// Command docgen writes the parts of the documentation that come from the code: docs/CLI.md from the
// commands' own help text, and the capability table in docs/PROVIDERS.md from
// internal/provider/capabilities.go.
//
//	go run ./internal/tools/docgen          write the files
//	go run ./internal/tools/docgen -check   fail when a file differs from what the code gives
//
// It also fails when a provider name or an engine tool is missing from the pages that describe it.
// go generate ./cmd/dummyrun runs it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/tools/repo"
)

func main() {
	check := flag.Bool("check", false, "fail when a generated file is out of date, and write nothing")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, "docgen:", err)
		os.Exit(1)
	}
}

func run(check bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := repo.Root(cwd)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "docgen-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // a leftover directory in the temp area is harmless
	bins, err := repo.Build(root, tmp)
	if err != nil {
		return err
	}
	cli, err := renderCLI(bins, root)
	if err != nil {
		return err
	}
	providersPath := filepath.Join(root, "docs", "PROVIDERS.md")
	providers, err := os.ReadFile(providersPath) //nolint:gosec // path built from the repository root
	if err != nil {
		return err
	}
	providersNew, err := replaceBetween(string(providers), renderProviderTable())
	if err != nil {
		return fmt.Errorf("docs/PROVIDERS.md: %w", err)
	}
	providersNew, err = replaceMarked(providersNew, defaultsBegin, defaultsEnd, renderDefaults())
	if err != nil {
		return fmt.Errorf("docs/PROVIDERS.md: %w", err)
	}
	files := map[string]string{
		filepath.Join(root, "docs", "CLI.md"): cli,
		providersPath:                         providersNew,
	}
	var problems []string
	problems = append(problems, checkNamed(root, providersNew)...)
	names := make([]string, 0, len(files))
	for p := range files {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		old, _ := os.ReadFile(p) //nolint:gosec // a missing file reads as empty and counts as out of date
		if string(old) == files[p] {
			continue
		}
		rel, _ := filepath.Rel(root, p)
		if check {
			problems = append(problems, rel+" is out of date. Run: go generate ./cmd/dummyrun")
			continue
		}
		if err := os.WriteFile(p, []byte(files[p]), 0o644); err != nil { //nolint:gosec // documentation is world-readable
			return err
		}
		fmt.Println("wrote", rel)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return nil
}

// checkNamed returns a problem for each provider name and engine tool that the pages describing them leave out.
func checkNamed(root, providersDoc string) []string {
	var out []string
	engine, err := os.ReadFile(filepath.Join(root, "docs", "ENGINE.md")) //nolint:gosec // fixed path
	if err != nil {
		return []string{err.Error()}
	}
	for _, name := range providerNames() {
		if !strings.Contains(providersDoc, "| `"+name+"` |") {
			out = append(out, "docs/PROVIDERS.md has no table row for the provider `"+name+"`")
		}
	}
	for _, tool := range record.Actions {
		if !strings.Contains(string(engine), "`"+tool+"`") {
			out = append(out, "docs/ENGINE.md does not mention the tool `"+tool+"`")
		}
	}
	return out
}
