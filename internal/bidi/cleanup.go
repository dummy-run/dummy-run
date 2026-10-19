package bidi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Profile directory prefixes. The engine makes these with os.MkdirTemp and
// nothing else uses them, so they say which processes and directories are its own.
const (
	chromeProfilePrefix  = "dummyrun-chrome-"
	firefoxProfilePrefix = "dummyrun-firefox-"
)

func isProfilePrefix(name string) bool {
	return strings.HasPrefix(name, chromeProfilePrefix) || strings.HasPrefix(name, firefoxProfilePrefix)
}

// live holds every browser that has been launched and not yet closed.
var live = struct {
	sync.Mutex
	set map[*Browser]struct{}
}{set: map[*Browser]struct{}{}}

func register(b *Browser) {
	live.Lock()
	live.set[b] = struct{}{}
	live.Unlock()
}

func unregister(b *Browser) {
	live.Lock()
	delete(live.set, b)
	live.Unlock()
}

// CloseAll closes every browser that is still open. A command calls it as it
// exits, so a path that skipped a deferred Close still leaves nothing behind.
func CloseAll() {
	live.Lock()
	all := make([]*Browser, 0, len(live.set))
	for b := range live.set {
		all = append(all, b)
	}
	live.Unlock()
	for _, b := range all {
		b.Close()
	}
}

// removeProfile removes a profile directory the engine made. It refuses any
// path that is not directly inside the temporary directory or lacks the prefix.
func removeProfile(dir string) error {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) || !isProfilePrefix(filepath.Base(dir)) || filepath.Dir(dir) != filepath.Clean(os.TempDir()) {
		return fmt.Errorf("bidi: refusing to remove %q: not an engine profile directory", dir)
	}
	var err error
	for i := 0; i < 10; i++ {
		// A dying browser can write into its profile for a moment, so try again.
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// stopProcsUsing ends every process whose command line names dir, which catches
// helpers that left the process group.
func stopProcsUsing(dir string) {
	procs, err := listProcesses()
	if err != nil {
		return
	}
	var pids []int
	for _, p := range procs {
		if strings.Contains(p.Args, dir) {
			pids = append(pids, p.PID)
		}
	}
	terminate(pids, 2*time.Second)
}

// terminate sends SIGTERM, waits, then kills what is left.
func terminate(pids []int, grace time.Duration) {
	for _, pid := range pids {
		termPID(pid)
	}
	waitGone(func() bool {
		for _, pid := range pids {
			if pidAlive(pid) {
				return true
			}
		}
		return false
	}, grace)
	for _, pid := range pids {
		if pidAlive(pid) {
			killPID(pid)
		}
	}
}

// Process is one running process.
type Process struct {
	PID  int
	Args string
}

func listProcesses() ([]Process, error) {
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- fixed command
	out, err := exec.CommandContext(context.Background(), "ps", "-axo", "pid=,args=").Output() //nolint:gosec // fixed command
	if err != nil {
		return nil, fmt.Errorf("bidi: list processes: %w", err)
	}
	var procs []Process
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		pid, rest, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(pid)
		if err != nil || n == selfPID() {
			continue
		}
		procs = append(procs, Process{PID: n, Args: strings.TrimSpace(rest)})
	}
	return procs, nil
}

// Leftovers is what a finished or crashed run left behind.
type Leftovers struct {
	// Processes are running processes whose command line names an engine profile directory.
	Processes []Process
	// Dirs are engine profile directories, with or without a process.
	Dirs []string
}

// profileDirs lists the engine profile directories in the temporary directory.
func profileDirs() []string {
	tmp := filepath.Clean(os.TempDir())
	var dirs []string
	for _, pre := range []string{chromeProfilePrefix, firefoxProfilePrefix} {
		m, _ := filepath.Glob(filepath.Join(tmp, pre+"*"))
		dirs = append(dirs, m...)
	}
	sort.Strings(dirs)
	return dirs
}

// FindLeftovers looks for engine browsers and profile directories. A browser
// that belongs to a run in progress is listed too, so check before stopping.
func FindLeftovers() (Leftovers, error) {
	var l Leftovers
	procs, err := listProcesses()
	if err != nil {
		return l, err
	}
	tmp := filepath.Clean(os.TempDir()) + string(os.PathSeparator)
	for _, p := range procs {
		if strings.Contains(p.Args, tmp+chromeProfilePrefix) || strings.Contains(p.Args, tmp+firefoxProfilePrefix) {
			l.Processes = append(l.Processes, p)
		}
	}
	l.Dirs = profileDirs()
	return l, nil
}

// Stop ends the listed processes and removes the listed directories.
func (l Leftovers) Stop() error {
	pids := make([]int, 0, len(l.Processes))
	for _, p := range l.Processes {
		pids = append(pids, p.PID)
	}
	terminate(pids, 3*time.Second)
	var errs []string
	for _, d := range l.Dirs {
		if err := removeProfile(d); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// ProfileDirs returns the temporary profile directories this browser owns.
func (b *Browser) ProfileDirs() []string { return append([]string(nil), b.tempDirs...) }

// Alive reports whether a process the browser started, or any of its children, is still running.
func (b *Browser) Alive() bool {
	for _, p := range b.procs {
		select {
		case <-p.done:
		default:
			return true
		}
		if groupAlive(p.cmd) {
			return true
		}
	}
	return false
}

// OpenBrowsers counts the browsers launched and not yet closed. A test binary
// checks it after its tests to find one that was never closed.
func OpenBrowsers() int {
	live.Lock()
	defer live.Unlock()
	return len(live.set)
}
