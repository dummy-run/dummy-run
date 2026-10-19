package identity

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGoldenVectors(t *testing.T) {
	data, err := os.ReadFile("testdata/fingerprint_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vec []struct {
		URL, LocusIn, Symptom, Route, Locus, Fingerprint string
	}
	var raw []map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, r := range raw {
		vec = append(vec, struct{ URL, LocusIn, Symptom, Route, Locus, Fingerprint string }{
			r["url"], r["locus_in"], r["symptom"], r["route"], r["locus"], r["fingerprint"]})
	}
	if len(vec) < 6 {
		t.Fatalf("only %d vectors", len(vec))
	}
	for _, v := range vec {
		if got := NormaliseRoute(v.URL); got != v.Route {
			t.Errorf("route(%q) = %q, want %q", v.URL, got, v.Route)
		}
		got, err := Fingerprint(v.URL, v.LocusIn, v.Symptom)
		if err != nil || got != v.Fingerprint {
			t.Errorf("fingerprint(%q, %q, %q) = %q, %v; want %q", v.URL, v.LocusIn, v.Symptom, got, err, v.Fingerprint)
		}
	}
}

func TestRouteNormalisation(t *testing.T) {
	for in, want := range map[string]string{
		"https://spektrabot.co.uk/docs/?b=2&a=1#x":              "/docs?a&b",
		"/users/12345/orders/":                                  "/users/:id/orders",
		"https://x.test/a/3f2b8c1e-1111-4222-8333-444455556666": "/a/:id",
		"https://x.test":                                        "/",
		"/Docs/Page":                                            "/Docs/Page",
		"/a/0123456789abcdef":                                   "/a/:id",
		"/a/abc":                                                "/a/abc",
		"/x?q=1&q=2&flag&&z=%41":                                "/x?flag&q&z",
		"//host.test/p?x#f":                                     "/p?x",
		"/users/:id/orders":                                     "/users/:id/orders",
		"/docs?a&b":                                             "/docs?a&b",
	} {
		if got := NormaliseRoute(in); got != want {
			t.Errorf("NormaliseRoute(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFingerprintIgnoresQueryValuesAndSlash(t *testing.T) {
	a, _ := Fingerprint("https://s.test/docs?page=1", "button:open menu", "off-screen")
	b, _ := Fingerprint("https://s.test/docs/?page=7#top", "button:open menu", "off-screen")
	c, _ := Fingerprint("https://s.test/docs", "button:open menu", "obscured")
	d, _ := Fingerprint("https://s.test/other", "button:open menu", "off-screen")
	if a != b || len(a) != 16 || a == c || a == d {
		t.Fatalf("a=%s b=%s c=%s d=%s", a, b, c, d)
	}
}

func TestFingerprintRejectsOpenSymptoms(t *testing.T) {
	for _, s := range []string{"looks weird", "missing-information", "off-screen:extra", "a11y", "missing-information:a-b-c-d-e", "missing-information:x\n"} {
		if _, err := Fingerprint("/", "page", s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	for _, s := range []string{"missing-information:uk-data-residency", "a11y:image-alt", "not-reachable"} {
		if !ValidSymptom(s) {
			t.Errorf("rejected %q", s)
		}
	}
}

func TestLocus(t *testing.T) {
	if got := NormaliseLocus("Button", "Open  menu!", ""); got != "button:open menu" {
		t.Error(got)
	}
	if got := NormaliseLocus("", "", "Main Nav"); got != "region:main nav" {
		t.Error(got)
	}
	if got := NormaliseLocus("", "", ""); got != "page" {
		t.Error(got)
	}
}

func TestSlug(t *testing.T) {
	if got := Slug("UK data residency, for the model!"); got != "uk-data-residency-for" {
		t.Error(got)
	}
	if !ValidSymptom("missing-information:" + Slug("Where do screenshots go?")) {
		t.Error("slug not valid")
	}
}

func FuzzNormaliseRoute(f *testing.F) {
	for _, s := range []string{"", "/", "https://x.test/a?b=c#d", "%", "/a/%zz?%=%", "\x00//\n:"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r := NormaliseRoute(s)
		if len(r) == 0 || r[0] != '/' {
			t.Fatalf("route %q", r)
		}
		if NormaliseRoute(r) != r {
			t.Fatalf("not idempotent: %q -> %q -> %q", s, r, NormaliseRoute(r))
		}
	})
}
