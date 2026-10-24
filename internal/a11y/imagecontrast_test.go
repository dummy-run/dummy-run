package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const imageContrastPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Banners</title>
<style>.hero{padding:30px;font-size:18px;color:#fff;width:600px}</style></head><body><main><h1>Banners</h1>
<p id="faint" class="hero" style="background:linear-gradient(90deg,#ddd,#fff)">Book your appointment online today</p>
<p id="strong" class="hero" style="background:linear-gradient(90deg,#123,#000)">Book your appointment online today</p>
</main></body></html>`

func TestImageContrastMeasuresTextOnGradients(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, imageContrastPage)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, ImageContrast: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" || !r.ImageContrastChecked {
				t.Fatalf("error %q checked %v", r.Error, r.ImageContrastChecked)
			}
			measured := map[string]bool{}
			for _, m := range r.ImageContrast {
				measured[m.Selector] = true
			}
			if !measured["#faint"] || !measured["#strong"] {
				t.Fatalf("both gradients should be measured: %+v (needs review %+v)", r.ImageContrast, r.NeedsReview)
			}
			var flagged []string
			for _, f := range r.Violations {
				if f.Rule == RuleImageContrast {
					for _, n := range f.Nodes {
						flagged = append(flagged, n.Target)
					}
				}
			}
			if strings.Join(flagged, ",") != "#faint" {
				t.Errorf("flagged %v, want #faint only: %+v", flagged, r.ImageContrast)
			}
		})
	}
}
