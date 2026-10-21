package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dummy-run/dummy-run/internal/browsers"
)

const browsersUsage = `Usage:
  dummyrun browsers install [chrome|firefox|all]
  dummyrun browsers list

install downloads the browsers Dummy Run was tested with into a cache folder of your own, so you do not match a
chromedriver to your Chrome by hand. "chrome" (the default) is Chrome for Testing with its chromedriver. "firefox"
uses the Firefox already on your machine, or downloads the pinned release on Linux. Each download is checked
against a SHA-256 hash that ships with Dummy Run. Running install again does nothing once a browser is in place.

list shows the versions Dummy Run pins, where each is installed, and the cache folder.

The cache is os.UserCacheDir()/dummyrun/browsers. DUMMYRUN_BROWSER_CACHE moves it. DUMMYRUN_CHROME,
DUMMYRUN_CHROMEDRIVER and DUMMYRUN_FIREFOX still win over the cache when you set them.`

func browsersCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, browsersUsage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprintln(os.Stderr, browsersUsage)
		return 0
	case "install":
		return browsersInstall(args[1:])
	case "list":
		return browsersList(args[1:])
	}
	fmt.Fprintf(os.Stderr, "dummyrun browsers: unknown command %q; want install or list\n", args[0])
	return 2
}

func browsersInstall(args []string) int {
	fs := flag.NewFlagSet("browsers install", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, browsersUsage) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	which := "chrome"
	switch fs.NArg() {
	case 0:
	case 1:
		which = fs.Arg(0)
	default:
		fmt.Fprintln(os.Stderr, "dummyrun browsers install: want one of chrome, firefox or all")
		return 2
	}
	if which != "chrome" && which != "firefox" && which != "all" {
		fmt.Fprintf(os.Stderr, "dummyrun browsers install: %q is not chrome, firefox or all\n", which)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	o := browsers.Options{Out: os.Stdout}
	failed := false
	if which == "chrome" || which == "all" {
		if _, _, err := browsers.InstallChrome(ctx, o); err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun:", err)
			failed = true
		}
	}
	if which == "firefox" || which == "all" {
		if _, err := browsers.InstallFirefox(ctx, o); err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun:", err)
			failed = true
		}
	}
	if failed {
		return 1
	}
	fmt.Println("Ready. Try it: dummyrun try https://your-site")
	return 0
}

func browsersList(args []string) int {
	fs := flag.NewFlagSet("browsers list", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, browsersUsage) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	pins := browsers.Pins()
	if root, err := browsers.CacheRoot(); err != nil {
		fmt.Printf("Cache folder: unknown (%v)\n\n", err)
	} else {
		fmt.Printf("Cache folder: %s\n\n", root)
	}
	fmt.Printf("%-9s %-14s %s\n", "Browser", "Pinned", "Where it is")
	if c, d, ok := browsers.CachedChrome(); ok {
		fmt.Printf("%-9s %-14s installed: %s\n%-24s with chromedriver %s\n", "chrome", pins.ChromeForTesting, c, "", d)
	} else {
		fmt.Printf("%-9s %-14s not installed. Run: dummyrun browsers install chrome\n", "chrome", pins.ChromeForTesting)
	}
	system := browsers.SystemFirefox()
	switch ff, ok := browsers.CachedFirefox(); {
	case ok:
		fmt.Printf("%-9s %-14s installed: %s\n", "firefox", pins.Firefox, ff)
	case system != "":
		fmt.Printf("%-9s %-14s using the Firefox on this machine: %s\n", "firefox", pins.Firefox, system)
	default:
		fmt.Printf("%-9s %-14s not found. Run: dummyrun browsers install firefox\n", "firefox", pins.Firefox)
	}
	for _, e := range []string{"DUMMYRUN_CHROME", "DUMMYRUN_CHROMEDRIVER", "DUMMYRUN_FIREFOX"} {
		if v := os.Getenv(e); v != "" {
			fmt.Printf("\n%s is set to %s and is used instead of the cache.\n", e, v)
		}
	}
	return 0
}
