//go:build unix

package bidi

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// ownGroup puts the child in a process group of its own, so one signal reaches
// the driver, the browser and every helper they start.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup sends sig to the child's whole group. The group ID equals the
// child's PID because ownGroup made it the leader.
func signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}

// groupAlive reports whether any process is left in the child's group.
func groupAlive(cmd *exec.Cmd) bool {
	if cmd.Process == nil {
		return false
	}
	return syscall.Kill(-cmd.Process.Pid, 0) == nil
}

func killGroup(cmd *exec.Cmd) { signalGroup(cmd, syscall.SIGKILL) }
func termGroup(cmd *exec.Cmd) { signalGroup(cmd, syscall.SIGTERM) }
func killPID(pid int)         { _ = syscall.Kill(pid, syscall.SIGKILL) }
func termPID(pid int)         { _ = syscall.Kill(pid, syscall.SIGTERM) }
func pidAlive(pid int) bool   { return syscall.Kill(pid, 0) == nil }
func selfPID() int            { return os.Getpid() }

// watchdogScript runs in a group of its own. When the engine process ($1)
// disappears, for example after SIGKILL or a crash in another goroutine, it
// stops the browser group ($2). It ends when either is gone.
const watchdogScript = `while kill -0 "$1" 2>/dev/null && kill -0 -- "-$2" 2>/dev/null; do sleep 1; done
if kill -0 -- "-$2" 2>/dev/null; then
  kill -TERM -- "-$2" 2>/dev/null
  sleep 2
  kill -KILL -- "-$2" 2>/dev/null
fi`

// startWatchdog starts the parent-death guard for the group led by cmd.
func startWatchdog(cmd *exec.Cmd) *exec.Cmd {
	if cmd.Process == nil {
		return nil
	}
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- fixed script and numeric arguments
	w := exec.CommandContext(context.Background(), "/bin/sh", "-c", watchdogScript, "sh", //nolint:gosec // fixed script, numeric arguments
		strconv.Itoa(os.Getpid()), strconv.Itoa(cmd.Process.Pid))
	ownGroup(w)
	if err := w.Start(); err != nil {
		return nil
	}
	return w
}

// waitGone polls until fn is false or the time is up.
func waitGone(fn func() bool, d time.Duration) bool {
	end := time.Now().Add(d)
	for fn() {
		if time.Now().After(end) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
	return true
}
