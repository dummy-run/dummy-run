package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const planTestPage = `<!doctype html><html lang="en"><head><title>Sam</title></head><body><h1>Hi, I'm Sam</h1><a href="/about">About</a></body></html>`

// protectedSite answers only requests that carry the preview header, as a protected preview does.
func protectedSite(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Preview") != "let-me-in" {
			http.Error(w, "preview is protected", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, planTestPage)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPlanRejectsAPersonaNotInTheLibrary(t *testing.T) {
	srv := protectedSite(t)
	out := filepath.Join(t.TempDir(), "cast")
	var stderr strings.Builder
	code := captureStderr(t, &stderr, func() int {
		return run([]string{"plan", srv.URL + "/", "--out", out, "--no-model", "--persona", "keyboard-only"})
	})
	if code != 2 {
		t.Errorf("plan exited %d, want 2 for a persona that is not in the library", code)
	}
	if !strings.Contains(stderr.String(), `"keyboard-only"`) || !strings.Contains(stderr.String(), "keyboard-only-user") {
		t.Errorf("the error should name the typo and the personas that exist:\n%s", stderr.String())
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused plan wrote a cast: %v", err)
	}
}

func TestPlanReadsAProtectedPreviewWithTheHeader(t *testing.T) {
	srv := protectedSite(t)
	t.Setenv("PLAN_TEST_PREVIEW", "let-me-in")
	out := filepath.Join(t.TempDir(), "cast")
	if code := run([]string{"plan", srv.URL + "/", "--out", out, "--no-model", "--header-env", "X-Preview:PLAN_TEST_PREVIEW"}); code != 0 {
		t.Fatalf("plan exited %d", code)
	}
	data, err := os.ReadFile(filepath.Join(out, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"title": "Sam"`) || strings.Contains(string(data), "let-me-in") {
		t.Errorf("recon should have read the protected page, and the secret must not be in plan.json:\n%s", data)
	}

	// Without the header the pages answer 401, and the plan says so.
	other := filepath.Join(t.TempDir(), "cast")
	if code := run([]string{"plan", srv.URL + "/", "--out", other, "--no-model"}); code != 0 {
		t.Fatalf("plan exited %d", code)
	}
	data, _ = os.ReadFile(filepath.Join(other, "plan.json"))
	if !strings.Contains(string(data), `"status": 401`) {
		t.Errorf("without the header the page should have been refused:\n%s", data)
	}
}

func TestMakePlanWritesNothingWhenStopped(t *testing.T) {
	srv := protectedSite(t)
	tgt, err := tryTarget(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	lib, err := libraryFS("")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := filepath.Join(t.TempDir(), "cast")
	pl, err := makePlan(ctx, planInput{Target: tgt, Library: lib, Personas: 1, Steps: 12, Out: out})
	if !errors.Is(err, context.Canceled) || pl != nil {
		t.Errorf("makePlan returned %v, %v; want context.Canceled and no plan", pl, err)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a stopped plan wrote a cast: %v", err)
	}
	if got := fail(err); got != 130 {
		t.Errorf("exit code = %d, want 130", got)
	}
}

// captureStderr runs f with os.Stderr sent to w.
func captureStderr(t *testing.T, w *strings.Builder, f func() int) int {
	t.Helper()
	r, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = pw
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			w.Write(buf[:n])
			if err != nil {
				break
			}
		}
		close(done)
	}()
	code := f()
	os.Stderr = old
	_ = pw.Close()
	<-done
	_ = r.Close()
	return code
}
