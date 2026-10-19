package bidi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// LaunchOptions says which browser to start and how.
type LaunchOptions struct {
	// Browser is "chrome" or "firefox".
	Browser string
	// Headless runs the browser without a window. Default true.
	Headless bool
	// UserAgent replaces the browser's user agent when set, at launch time.
	UserAgent string
	// ChromePath is the Chrome binary. Env DUMMYRUN_CHROME, then a platform default.
	ChromePath string
	// ChromedriverPath is the chromedriver binary. Env DUMMYRUN_CHROMEDRIVER, then PATH.
	ChromedriverPath string
	// FirefoxPath is the Firefox binary. Env DUMMYRUN_FIREFOX, then a platform default.
	FirefoxPath string
	// Timeout bounds the launch. Default 60 seconds.
	Timeout time.Duration
	// Env is appended to the browser's environment.
	Env []string
	// Args are extra command-line arguments for the browser, appended after the defaults.
	Args []string
	// FirefoxPrefs are extra Firefox preferences, written to the temporary profile.
	FirefoxPrefs map[string]any
}

// Browser is a launched browser with its BiDi connection.
type Browser struct {
	Conn *Conn
	// Name is "chrome" or "firefox".
	Name string
	// Version is the browser version the session reported.
	Version string
	// UserAgent is navigator.userAgent of the first context.
	UserAgent string
	procs     []*exec.Cmd
	tempDirs  []string
}

// Launch starts the browser and connects to it.
func Launch(ctx context.Context, opts LaunchOptions) (*Browser, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	switch opts.Browser {
	case "firefox":
		return launchFirefox(ctx, opts)
	case "chrome":
		return launchChrome(ctx, opts)
	default:
		return nil, fmt.Errorf("bidi: unknown browser %q (chrome or firefox)", opts.Browser)
	}
}

// Close ends the session, stops the processes and removes temporary profiles.
func (b *Browser) Close() {
	if b.Conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = b.Conn.BrowserClose(ctx)
		cancel()
		_ = b.Conn.Close()
	}
	for _, p := range b.procs {
		if p.Process != nil {
			_ = p.Process.Kill()
			_ = p.Wait()
		}
	}
	for _, d := range b.tempDirs {
		_ = os.RemoveAll(d)
	}
}

func freePort() (int, error) {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	return port, l.Close()
}

func firstExisting(candidates ...string) string {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		// Only a path is checked on disk. A bare name goes through exec.LookPath, which
		// refuses to resolve it against the current directory.
		if filepath.IsAbs(c) || strings.ContainsRune(c, os.PathSeparator) {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() { //nolint:gosec // candidate paths are configuration, not user input
				return c
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

// firefoxPrefs quieten first-run and update behaviour in the temporary profile.
var firefoxPrefs = map[string]any{
	"browser.shell.checkDefaultBrowser":          false,
	"browser.aboutwelcome.enabled":               false,
	"browser.startup.homepage":                   "about:blank",
	"startup.homepage_welcome_url":               "about:blank",
	"browser.newtabpage.enabled":                 false,
	"app.update.disabledForTesting":              true,
	"app.update.auto":                            false,
	"datareporting.policy.dataSubmissionEnabled": false,
	"datareporting.healthreport.uploadEnabled":   false,
	"toolkit.telemetry.reportingpolicy.firstRun": false,
	"signon.rememberSignons":                     false,
	"browser.tabs.warnOnClose":                   false,
	"browser.warnOnQuit":                         false,
	"remote.active-protocols":                    1, // BiDi only
}

func launchFirefox(ctx context.Context, opts LaunchOptions) (*Browser, error) {
	bin := firstExisting(opts.FirefoxPath, os.Getenv("DUMMYRUN_FIREFOX"),
		"/Applications/Firefox.app/Contents/MacOS/firefox", "firefox", "firefox-esr")
	if bin == "" {
		return nil, errors.New("bidi: firefox not found; set DUMMYRUN_FIREFOX")
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp("", "dummyrun-firefox-")
	if err != nil {
		return nil, err
	}
	b := &Browser{Name: "firefox", tempDirs: []string{profile}}
	prefs := make(map[string]any, len(firefoxPrefs)+1)
	for k, v := range firefoxPrefs {
		prefs[k] = v
	}
	if opts.UserAgent != "" {
		prefs["general.useragent.override"] = opts.UserAgent
	}
	for k, v := range opts.FirefoxPrefs {
		prefs[k] = v
	}
	var userJS strings.Builder
	for k, v := range prefs {
		val, _ := json.Marshal(v)
		fmt.Fprintf(&userJS, "user_pref(%q, %s);\n", k, val)
	}
	if err := os.WriteFile(filepath.Join(profile, "user.js"), []byte(userJS.String()), 0o600); err != nil {
		b.Close()
		return nil, err
	}
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--profile", profile, "--no-remote",
		"--remote-allow-system-access",
	}
	if opts.Headless {
		args = append(args, "--headless")
	}
	args = append(args, opts.Args...)
	args = append(args, "about:blank")
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- the binary path is the operator's configuration, never page or model output
	cmd := exec.CommandContext(context.Background(), bin, args...) //nolint:gosec // see above
	cmd.Env = append(os.Environ(), opts.Env...)
	if runtime.GOOS == "linux" {
		// A clean config home keeps the system Firefox's settings out of the run.
		cmd.Env = append(cmd.Env, "XDG_CONFIG_HOME="+profile)
		if os.Getenv("CI") != "" {
			// The GitHub runner's user namespaces stop the content sandbox from starting.
			cmd.Env = append(cmd.Env, "MOZ_DISABLE_CONTENT_SANDBOX=1")
		}
	}
	if err := cmd.Start(); err != nil {
		b.Close()
		return nil, fmt.Errorf("bidi: start firefox: %w", err)
	}
	b.procs = append(b.procs, cmd)
	url := fmt.Sprintf("ws://127.0.0.1:%d/session", port)
	conn, err := dialUntilReady(ctx, url)
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("bidi: firefox did not open its BiDi port: %w", err)
	}
	b.Conn = conn
	res, err := conn.SessionNew(ctx, Capabilities{AlwaysMatch: map[string]any{
		"browserName":             "firefox",
		"webSocketUrl":            true,
		"unhandledPromptBehavior": map[string]any{"default": "ignore"},
	}})
	if err != nil {
		b.Close()
		return nil, err
	}
	b.Version, _ = res.Capabilities["browserVersion"].(string)
	return b, nil
}

func dialUntilReady(ctx context.Context, url string) (*Conn, error) {
	for {
		dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		c, err := Dial(dctx, url)
		cancel()
		if err == nil {
			return c, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func launchChrome(ctx context.Context, opts LaunchOptions) (*Browser, error) {
	driver := firstExisting(opts.ChromedriverPath, os.Getenv("DUMMYRUN_CHROMEDRIVER"), "chromedriver")
	if driver == "" {
		return nil, errors.New("bidi: chromedriver not found; set DUMMYRUN_CHROMEDRIVER")
	}
	chrome := firstExisting(opts.ChromePath, os.Getenv("DUMMYRUN_CHROME"),
		"/Applications/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"google-chrome", "chrome", "chromium")
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	userData, err := os.MkdirTemp("", "dummyrun-chrome-")
	if err != nil {
		return nil, err
	}
	b := &Browser{Name: "chrome", tempDirs: []string{userData}}
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- the binary path is the operator's configuration, never page or model output
	cmd := exec.CommandContext(context.Background(), driver, fmt.Sprintf("--port=%d", port)) //nolint:gosec // see above
	cmd.Env = append(os.Environ(), opts.Env...)
	if err := cmd.Start(); err != nil {
		b.Close()
		return nil, fmt.Errorf("bidi: start chromedriver: %w", err)
	}
	b.procs = append(b.procs, cmd)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitHTTP(ctx, base+"/status"); err != nil {
		b.Close()
		return nil, fmt.Errorf("bidi: chromedriver did not answer: %w", err)
	}
	args := []string{"--user-data-dir=" + userData, "--no-first-run", "--no-default-browser-check",
		"--disable-background-networking", "--disable-sync", "--window-size=1280,900"}
	if opts.Headless {
		args = append(args, "--headless=new")
	}
	if opts.UserAgent != "" {
		args = append(args, "--user-agent="+opts.UserAgent)
	}
	if os.Getenv("CI") != "" {
		args = append(args, "--no-sandbox", "--disable-dev-shm-usage")
	}
	args = append(args, opts.Args...)
	chromeOpts := map[string]any{"args": args}
	if chrome != "" {
		chromeOpts["binary"] = chrome
	}
	body, _ := json.Marshal(map[string]any{"capabilities": map[string]any{"alwaysMatch": map[string]any{
		"browserName":             "chrome",
		"webSocketUrl":            true,
		"unhandledPromptBehavior": map[string]any{"default": "ignore"},
		"goog:chromeOptions":      chromeOpts,
	}}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("bidi: chromedriver session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var v struct {
		Value struct {
			SessionID    string         `json:"sessionId"`
			Capabilities map[string]any `json:"capabilities"`
			Error        string         `json:"error"`
			Message      string         `json:"message"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		b.Close()
		return nil, fmt.Errorf("bidi: chromedriver session reply: %w", err)
	}
	if v.Value.Error != "" {
		b.Close()
		return nil, fmt.Errorf("bidi: chromedriver: %s: %s", v.Value.Error, v.Value.Message)
	}
	wsURL, _ := v.Value.Capabilities["webSocketUrl"].(string)
	if wsURL == "" {
		b.Close()
		return nil, errors.New("bidi: chromedriver returned no webSocketUrl")
	}
	conn, err := Dial(ctx, wsURL)
	if err != nil {
		b.Close()
		return nil, err
	}
	b.Conn = conn
	b.Version, _ = v.Value.Capabilities["browserVersion"].(string)
	return b, nil
}

func waitHTTP(ctx context.Context, url string) error {
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
