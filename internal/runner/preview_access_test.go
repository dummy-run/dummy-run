package runner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/access/accesstest"
	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/runner"
)

// A variable that is not set stops the run before the browser starts, and names the variable only.
func TestPreviewAccessNeedsItsVariables(t *testing.T) {
	fx := accesstest.New(t)
	persona, err := cast.Load(writeRoleCast(t, "viewer"), "viewer")
	if err != nil {
		t.Fatal(err)
	}
	_, err = runner.Run(context.Background(), runner.Config{Persona: persona, Scenario: persona.Scenarios[0],
		Provider: &scripted{}, Browser: "chrome", Headless: true, OutDir: t.TempDir(), Target: fx.Target(accesstest.Auth()),
		Getenv: func(n string) string {
			if n == "PREVIEW_USER" {
				return accesstest.User
			}
			return ""
		}})
	if err == nil || !strings.Contains(err.Error(), "PREVIEW_PASS") {
		t.Fatalf("want an error naming PREVIEW_PASS, got %v", err)
	}
	noSecrets(t, "error", []byte(err.Error()), accesstest.Secrets())
	if n := len(fx.Preview.Requests()); n != 0 {
		t.Errorf("the preview got %d requests before the run stopped", n)
	}
}

// TestPreviewAccessEndToEnd runs a persona against the protected fixture on each browser in
// DUMMYRUN_BROWSERS. The preview wants Basic auth on /basic, a bypass header on /bypass and both on
// /both. The read-only policy stays on, the redirect to another host is blocked and gets nothing, and
// no output holds a secret.
func TestPreviewAccessEndToEnd(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the protected preview test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		t.Run(browser, func(t *testing.T) {
			fx := accesstest.New(t)
			persona, err := cast.Load(writeRoleCast(t, "viewer"), "viewer")
			if err != nil {
				t.Fatal(err)
			}
			prov := &scripted{calls: []provider.ToolCall{
				call("visit", `{"url":"/both"}`),
				call("visit", `{"url":"/basic"}`),
				call("visit", `{"url":"/bypass"}`),
				call("visit", `{"url":"/form"}`),
				call("click", `{"target":"Send it"}`),
				call("visit", `{"url":"/redirect"}`),
			}}
			out := t.TempDir()
			var log bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			res, err := runner.Run(ctx, runner.Config{Persona: persona, Scenario: persona.Scenarios[0], Provider: prov,
				Browser: browser, Headless: true, OutDir: out, Target: fx.Target(accesstest.Auth()), Getenv: accesstest.Getenv, Log: &log})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			rec := res.Record
			for _, path := range []string{"/both", "/basic", "/bypass", "/form"} {
				if got := stepStatus(rec, path); got != http.StatusOK {
					t.Errorf("%s status %d, want 200", path, got)
				}
			}
			pa := rec.PreviewAccess
			if pa == nil || strings.Join(pa.Methods, ",") != "basic,headers" || strings.Join(pa.HeaderNames, ",") != accesstest.BypassHeader {
				t.Errorf("preview_access %+v", pa)
			}
			if n := len(fx.Preview.For("/submit")); n != 0 {
				t.Errorf("%d POSTs reached /submit; the read-only policy was bypassed", n)
			}
			if n := len(fx.Elsewhere.Requests()); n != 0 {
				t.Errorf("the other host got %d requests; the redirect should be blocked and carry nothing", n)
			}
			if !strings.Contains(log.String(), "access: protected preview") {
				t.Errorf("the log does not say which access was used:\n%s", log.String())
			}

			// Nothing the engine wrote, logged or sent to the model holds a secret.
			reqJSON, _ := json.Marshal(prov.requests)
			secrets := accesstest.Secrets()
			noSecrets(t, "model requests", reqJSON, secrets)
			noSecrets(t, "output files", readAll(t, out), secrets)
			noSecrets(t, "log", log.Bytes(), secrets)
			raw, _ := json.Marshal(rec)
			noSecrets(t, "record", raw, secrets)
		})
	}
}
