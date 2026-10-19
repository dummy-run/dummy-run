package watch

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func collect(t *testing.T, p *Poller, stop func(seen []string) bool) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	var seen []string
	done := make(chan struct{})
	go func() {
		_ = p.Run(ctx, func(path string) {
			mu.Lock()
			seen = append(seen, filepath.Base(path))
			s := append([]string(nil), seen...)
			mu.Unlock()
			if stop(s) {
				cancel()
			}
		})
		close(done)
	}()
	<-done
	mu.Lock()
	defer mu.Unlock()
	return seen
}

func TestNewFilesAreHandledOnceStable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &Poller{Dir: dir, Interval: 20 * time.Millisecond}
	go func() {
		time.Sleep(60 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "new.json"), []byte("{}"), 0o600)
		_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	}()
	seen := collect(t, p, func(s []string) bool { return len(s) >= 1 })
	if len(seen) != 1 || seen[0] != "new.json" {
		t.Fatalf("handled %v, want only new.json", seen)
	}
}

func TestExistingFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"b.json", "a.json"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := &Poller{Dir: dir, Interval: 20 * time.Millisecond, IncludeExisting: true}
	seen := collect(t, p, func(s []string) bool { return len(s) >= 2 })
	if len(seen) != 2 || seen[0] != "a.json" || seen[1] != "b.json" {
		t.Fatalf("handled %v, want a.json then b.json", seen)
	}
}

func TestEachFileIsHandledOnce(t *testing.T) {
	dir := t.TempDir()
	p := &Poller{Dir: dir, Interval: 10 * time.Millisecond, IncludeExisting: true}
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	count := 0
	seen := collect(t, p, func(_ []string) bool {
		count++
		// Keep polling for a while after the first hand-over to catch repeats.
		time.Sleep(50 * time.Millisecond)
		return true
	})
	if len(seen) != 1 || count != 1 {
		t.Fatalf("handled %v %d times, want once", seen, count)
	}
}
