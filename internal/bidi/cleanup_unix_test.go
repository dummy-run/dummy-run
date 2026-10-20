//go:build unix

package bidi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeBrowser starts a shell that leaves a grandchild running, as a driver does
// with its browser. The grandchild's PID is written to a file.
func fakeBrowser(t *testing.T) (*Browser, string, int) {
	t.Helper()
	profile, err := os.MkdirTemp("", chromeProfilePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "Cookies"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	b := &Browser{Name: "chrome", tempDirs: []string{profile}}
	register(b)
	t.Cleanup(b.Close)
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", //nolint:gosec // test script
		`sleep 300 & echo $! > "$2"; wait`, "sh", profile, pidFile)
	if err := b.start(cmd); err != nil {
		t.Fatal(err)
	}
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(pidFile); err == nil { //nolint:gosec // test file
			if n, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
				pid = n
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("the fake browser did not start its child")
	}
	return b, profile, pid
}

func TestCloseStopsTheGroupAndRemovesTheProfile(t *testing.T) {
	b, profile, child := fakeBrowser(t)
	if !b.Alive() || !pidAlive(child) {
		t.Fatal("the fake browser should be running")
	}
	b.Close()
	if b.Alive() {
		t.Error("a process of the browser is still running")
	}
	if waitGone(func() bool { return pidAlive(child) }, 2*time.Second) == false {
		t.Errorf("child %d outlived Close", child)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Errorf("profile %s was not removed", profile)
	}
	b.Close() // a second call must be harmless
	live.Lock()
	_, still := live.set[b]
	live.Unlock()
	if still {
		t.Error("a closed browser is still registered")
	}
}

func TestCloseAllStopsEveryOpenBrowser(t *testing.T) {
	b1, p1, c1 := fakeBrowser(t)
	b2, p2, c2 := fakeBrowser(t)
	CloseAll()
	for _, b := range []*Browser{b1, b2} {
		if b.Alive() {
			t.Error("a browser survived CloseAll")
		}
	}
	for i, pid := range []int{c1, c2} {
		if !waitGone(func() bool { return pidAlive(pid) }, 2*time.Second) {
			t.Errorf("child %d (browser %d) outlived CloseAll", pid, i+1)
		}
	}
	for _, p := range []string{p1, p2} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("profile %s was not removed", p)
		}
	}
}

// A helper that left the process group is still found by the profile path on its command line.
func TestCloseStopsAHelperOutsideTheGroup(t *testing.T) {
	b, profile, _ := fakeBrowser(t)
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- test helper
	stray := exec.CommandContext(context.Background(), "/bin/sh", "-c", "sleep 300; true", "sh", "--user-data-dir="+profile) //nolint:gosec // test helper
	ownGroup(stray)
	if err := stray.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killGroup(stray) }) // the shell's own sleep
	reaped := make(chan struct{})
	go func() { _ = stray.Wait(); close(reaped) }()
	b.Close()
	select {
	case <-reaped:
	case <-time.After(5 * time.Second):
		_ = stray.Process.Kill()
		t.Fatal("a helper outside the group outlived Close")
	}
}

func TestWatchdogStopsTheGroupWhenTheOwnerIsGone(t *testing.T) {
	// The owner is a short-lived process, standing in for an engine that was killed.
	owner := exec.CommandContext(context.Background(), "sleep", "1")
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	victim := exec.CommandContext(context.Background(), "sleep", "300")
	ownGroup(victim)
	if err := victim.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := make(chan struct{})
	go func() { _ = victim.Wait(); close(reaped) }()
	wd := exec.CommandContext(context.Background(), "/bin/sh", "-c", watchdogScript, "sh", //nolint:gosec // fixed script
		strconv.Itoa(owner.Process.Pid), strconv.Itoa(victim.Process.Pid))
	ownGroup(wd)
	if err := wd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = victim.Process.Kill(); _ = wd.Process.Kill(); _ = wd.Wait() })
	go func() { _ = owner.Wait() }()
	select {
	case <-reaped:
	case <-time.After(15 * time.Second):
		t.Fatal("the watchdog did not stop the group after its owner exited")
	}
}

func TestLeftoversFindsAndStopsAnEngineProcess(t *testing.T) {
	profile, err := os.MkdirTemp("", firefoxProfilePrefix)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "sleep 300; true", "sh", "--profile="+profile) //nolint:gosec // test helper
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := make(chan struct{})
	go func() { _ = cmd.Wait(); close(reaped) }()
	t.Cleanup(func() { killGroup(cmd); _ = removeProfile(profile) })
	l, err := FindLeftovers()
	if err != nil {
		t.Fatal(err)
	}
	var mine Leftovers
	for _, p := range l.Processes {
		if p.PID == cmd.Process.Pid {
			mine.Processes = append(mine.Processes, p)
		}
	}
	if len(mine.Processes) != 1 {
		t.Fatalf("process %d not found among %d", cmd.Process.Pid, len(l.Processes))
	}
	mine.Dirs = []string{profile} // stop only what this test made
	if err := mine.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reaped:
	case <-time.After(5 * time.Second):
		t.Fatal("the leftover process was not stopped")
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Error("the profile was not removed")
	}
}
