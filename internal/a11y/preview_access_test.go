package a11y

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/access/accesstest"
)

// TestPreviewAccessInBrowser sweeps pages behind Basic auth and a bypass header on each browser in
// DUMMYRUN_BROWSERS. The sweep must get in, the redirect to another host must carry nothing, and neither
// the report nor the log may hold a secret.
func TestPreviewAccessInBrowser(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser test")
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			fx := accesstest.New(t)
			acc, err := access.Resolve(accesstest.Auth(), accesstest.Getenv, access.OriginScope([]string{fx.Preview.URL}))
			if err != nil {
				t.Fatal(err)
			}
			var log bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{
				URLs:      []string{fx.Preview.URL + "/both", fx.Preview.URL + "/redirect"},
				Browser:   browser,
				Viewports: []Viewport{Desktop},
				Schemes:   []string{Light},
				Headless:  true,
				Log:       &log,
				Access:    acc,
				Failures:  true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Summary.PageErrors != 0 {
				t.Errorf("%d pages could not be tested:\n%s", rep.Summary.PageErrors, log.String())
			}
			for _, r := range rep.Results {
				if r.FailureProbe != nil || !strings.Contains(r.FailureProbeError, "preview access") {
					t.Errorf("%s: the failure probe ran with preview access: %+v, %q", r.URL, r.FailureProbe, r.FailureProbeError)
				}
			}
			if rep.PreviewAccess == nil || strings.Join(rep.PreviewAccess.Methods, ",") != "basic,headers" {
				t.Errorf("preview_access %+v", rep.PreviewAccess)
			}
			if len(fx.Preview.For("/both")) == 0 || fx.Preview.For("/both")[len(fx.Preview.For("/both"))-1].Header.Get(accesstest.BypassHeader) != accesstest.Bypass {
				t.Error("the page behind the protection was not sent the header")
			}
			landing := fx.Elsewhere.For("/landing")
			if len(landing) == 0 {
				t.Fatal("the other host saw no request, so the redirect proved nothing")
			}
			for _, r := range landing {
				if r.Header.Get("Authorization") != "" || r.Header.Get(accesstest.BypassHeader) != "" {
					t.Error("the other host received the credentials")
				}
			}
			raw, _ := json.Marshal(rep)
			for _, s := range accesstest.Secrets() {
				if bytes.Contains(raw, []byte(s)) || bytes.Contains(log.Bytes(), []byte(s)) {
					t.Errorf("the report or log holds a secret (%d characters)", len(s))
				}
			}
		})
	}
}
