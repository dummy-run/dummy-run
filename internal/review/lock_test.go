//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows

package review

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAnEditWaitsForAnotherReviewerWhoHoldsTheFolder(t *testing.T) {
	dir := copyExamples(t)
	path := filepath.Join(dir, "2026-10-15-sam-author-001.json")
	unlock, err := lockDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := editFile(path, fpLone, Decision{Verdict: Confirmed, Reviewer: "R", At: when})
		done <- err
	}()
	select {
	case <-done:
		unlock()
		t.Fatal("the edit went ahead while another reviewer held the folder")
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
