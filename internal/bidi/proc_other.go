//go:build !unix

package bidi

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// There are no process groups here. taskkill with /T ends the whole tree.

func ownGroup(*exec.Cmd) {}

func taskkill(pid int) {
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- fixed command and a numeric argument
	_ = exec.CommandContext(context.Background(), "taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run() //nolint:gosec // fixed command, numeric argument
}

func groupAlive(cmd *exec.Cmd) bool { return cmd.ProcessState == nil }
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		taskkill(cmd.Process.Pid)
	}
}
func termGroup(cmd *exec.Cmd)           { killGroup(cmd) }
func killPID(pid int)                   { taskkill(pid) }
func termPID(pid int)                   { taskkill(pid) }
func pidAlive(int) bool                 { return false }
func selfPID() int                      { return os.Getpid() }
func startWatchdog(*exec.Cmd) *exec.Cmd { return nil }

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
