package a11y

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPlainLanguageSkipsAHiddenMain checks that a page whose main landmark is hidden counts as checked and never
// reaches Vale, on each browser in DUMMYRUN_BROWSERS. The Vale here is a script that fails if it runs.
func TestPlainLanguageSkipsAHiddenMain(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser test")
	}
	vale := filepath.Join(t.TempDir(), "vale")
	if err := os.WriteFile(vale, []byte("#!/bin/sh\necho 'vale ran' >&2\nexit 3\n"), 0o700); err != nil { //nolint:gosec // a test script that must be executable
		t.Fatal(err)
	}
	t.Setenv("DUMMYRUN_VALE", vale)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Hidden</title></head>
<body><main style="opacity:0"><h1>Hidden</h1><p>Leverage synergies going forward.</p></main></body></html>`)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			var log bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Log: &log, PlainLanguage: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if !r.PlainLanguageChecked || r.PlainLanguageError != "" {
				t.Errorf("checked %v, error %q, want checked with no error:\n%s", r.PlainLanguageChecked, r.PlainLanguageError, log.String())
			}
		})
	}
}
