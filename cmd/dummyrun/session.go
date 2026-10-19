package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/signin"
)

const sessionUsage = `Usage:
  dummyrun session save --target CONFIG --role ROLE --out FILE [--browser chrome|firefox] [--headful]

Signs in once through the login form named in the target file, with the credentials in the role's
environment variables, and writes the browser's cookies and local storage to FILE. The file is in
Playwright's storageState format. "dummyrun run --session FILE" and "dummyrun a11y --session FILE" load it
instead of signing in, so CI can reuse one sign-in.

The file holds live session cookies. It is written with mode 0600. Keep it out of version control and
treat it like a password. It prints nothing but a count.

Flags:
  --target CONFIG   the target file; it needs a signin block
  --role ROLE       which of the target's credentials roles to sign in as
  --out FILE        where to write the session
  --browser NAME    chrome or firefox (default chrome)
  --headful         show the browser window`

func sessionCmd(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stderr, sessionUsage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if args[0] != "save" {
		fmt.Fprintf(os.Stderr, "dummyrun session: unknown command %q; want \"save\"\n", args[0])
		return 2
	}
	return sessionSave(args[1:])
}

func sessionSave(args []string) int {
	fs := flag.NewFlagSet("session save", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, sessionUsage) }
	targetPath := fs.String("target", "", "target file with a signin block")
	role := fs.String("role", "", "credentials role to sign in as")
	out := fs.String("out", "", "session file to write")
	browser := fs.String("browser", "chrome", "chrome or firefox")
	headful := fs.Bool("headful", false, "show the browser window")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *targetPath == "" || *role == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "dummyrun session save: --target, --role and --out are required")
		fmt.Fprintln(os.Stderr, sessionUsage)
		return 2
	}
	if *browser != "chrome" && *browser != "firefox" {
		fmt.Fprintf(os.Stderr, "dummyrun session save: --browser %q: want chrome or firefox\n", *browser)
		return 2
	}
	target, err := loadTarget(*targetPath)
	if err != nil {
		return fail(err)
	}
	plan, err := signin.ResolveForm(target, *role, os.Getenv)
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	gate := policy.NewGate(target)
	p, err := page.Open(ctx, page.Options{Browser: *browser, Headless: !*headful, Width: 1280, Height: 900,
		RequestHook: signin.Hook(target, gate)})
	if err != nil {
		return fail(fmt.Errorf("open browser: %w", err))
	}
	defer p.Close()
	n, err := signin.SaveForm(ctx, p, target, plan, gate, *out)
	if err != nil {
		return fail(errors.New(signin.Scrub(err.Error(), plan.Secrets()...)))
	}
	fmt.Printf("saved %d cookies for the role %s to %s (mode 0600)\n", n, plan.Role, *out)
	return 0
}
