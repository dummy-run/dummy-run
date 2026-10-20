// Command doctest runs the shell code blocks in the documentation that are marked with a comment on the
// line before them:
//
//	<!-- doctest -->
//	<!-- doctest site -->
//
// The commands run against dummyrun and dummyrun-otel built from the working tree, from the repository
// root. Only safe commands are allowed: version, help, dummyrun-otel export --dry-run, dummyrun coverage
// without --out, and dummyrun a11y on a site served on this machine. A block marked "site" needs the built site served on port 1313 and
// runs only with -site. Use it from CI after the site is built.
//
//	go run ./internal/tools/doctest         run the blocks that need nothing else
//	go run ./internal/tools/doctest -site   run all of them, including the ones that need the local site
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/tools/repo"
)

func main() {
	site := flag.Bool("site", false, "also run the blocks marked \"site\", which need the built site served on port 1313")
	flag.Parse()
	if err := run(*site); err != nil {
		fmt.Fprintln(os.Stderr, "doctest:", err)
		os.Exit(1)
	}
}

func run(withSite bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := repo.Root(cwd)
	if err != nil {
		return err
	}
	blocks, err := collect(root)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return fmt.Errorf("no marked blocks found, so the documented commands are not tested")
	}
	tmp, err := os.MkdirTemp("", "doctest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // a leftover directory in the temp area is harmless
	bins, err := repo.Build(root, tmp)
	if err != nil {
		return err
	}
	failed := 0
	for _, b := range blocks {
		where := fmt.Sprintf("%s:%d", b.File, b.Line)
		if b.Site && !withSite {
			fmt.Println("skip", where, "(needs the served site, use -site)")
			continue
		}
		if err := runBlock(root, bins.Dir, b); err != nil {
			fmt.Printf("FAIL %s\n%v\n", where, err)
			failed++
			continue
		}
		fmt.Println("ok  ", where)
	}
	if failed > 0 {
		return fmt.Errorf("%d documented command block(s) failed", failed)
	}
	return nil
}

// collect reads every Markdown file under root except dependencies and build output.
func collect(root string) ([]block, error) {
	var all []block
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "public", "resources", ".playwright-mcp":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // walking the repository
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		bs, err := extract(rel, string(data))
		if err != nil {
			return err
		}
		all = append(all, bs...)
		return nil
	})
	return all, err
}

func runBlock(root, binDir string, b block) error {
	cmds := commands(b.Code)
	if len(cmds) == 0 {
		return fmt.Errorf("the block has no commands")
	}
	for _, c := range cmds {
		if err := checkSafe(c); err != nil {
			return err
		}
	}
	timeout := 2 * time.Minute
	if b.Site {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, c := range cmds {
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		cmd := exec.CommandContext(ctx, "sh", "-c", c) //nolint:gosec // checkSafe allowed this line
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("$ %s\n%s%w", c, tail(out.String()), err)
		}
	}
	return nil
}

// tail keeps the last 40 lines of output for a failure message.
func tail(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	return strings.Join(lines, "\n") + "\n"
}
