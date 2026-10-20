package robots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseGroups(t *testing.T) {
	file := "# comment\nUser-agent: *\nDisallow: /private/\n\nUser-agent: DummyRun\nDisallow: /nope\nAllow: /nope/yes\n\nUser-agent: other\nDisallow: /\n"
	dr := Parse([]byte(file), "DummyRun")
	if dr.Group != "dummyrun" {
		t.Fatalf("group %q", dr.Group)
	}
	cases := map[string]bool{"/": true, "/private/x": true, "/nope": false, "/nope/x": false, "/nope/yes": true, "/robots.txt": true}
	for p, want := range cases {
		if got := dr.Allowed(p); got != want {
			t.Errorf("DummyRun Allowed(%q) = %v, want %v", p, got, want)
		}
	}
	// A visitor with no group of its own gets the * rules, not the other agents'.
	star := Parse([]byte(file), "Unknown")
	if star.Group != "*" || star.Allowed("/private/x") || !star.Allowed("/nope") {
		t.Errorf("star group %+v", star)
	}
}

func TestTokenIsCaseInsensitiveAndMerged(t *testing.T) {
	file := "user-agent: DUMMYRUN\ndisallow: /a\n\nUser-agent: dummyrun\nDisallow: /b\n"
	r := Parse([]byte(file), "DummyRun")
	if r.Allowed("/a") || r.Allowed("/b") || !r.Allowed("/c") {
		t.Errorf("merge failed %+v", r.rules)
	}
}

func TestSharedGroupAndRuleEndsAgentList(t *testing.T) {
	file := "User-agent: bot1\nUser-agent: DummyRun\nDisallow: /x\nUser-agent: late\nDisallow: /y\n"
	r := Parse([]byte(file), "DummyRun")
	if r.Allowed("/x") || !r.Allowed("/y") {
		t.Errorf("shared group %+v", r.rules)
	}
}

func TestLongestMatchAndAllowTie(t *testing.T) {
	r := Parse([]byte("User-agent: *\nDisallow: /a\nAllow: /a/b\nDisallow: /a/b/c\nAllow: /t\nDisallow: /t\n"), "DummyRun")
	for p, want := range map[string]bool{"/a": false, "/a/b": true, "/a/b/x": true, "/a/b/c": false, "/t": true} {
		if got := r.Allowed(p); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestWildcardsAndAnchors(t *testing.T) {
	r := Parse([]byte("User-agent: *\nDisallow: /*.pdf$\nDisallow: /search?*q=\nDisallow: /tmp*/cache\nAllow: /\n"), "DummyRun")
	for p, want := range map[string]bool{
		"/a.pdf": false, "/a.pdf?x=1": true, "/dir/a.pdf": false, "/a.pdfx": true,
		"/search?page=2&q=1": false, "/search?page=2": true, "/tmp1/cache": false, "/tmp/x/cache/y": false, "/tmp/y": true,
	} {
		if got := r.Allowed(p); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestEmptyDisallowAndNoFile(t *testing.T) {
	if !Parse([]byte("User-agent: *\nDisallow:\n"), "DummyRun").Allowed("/anything") {
		t.Error("empty Disallow should allow everything")
	}
	if !Parse(nil, "DummyRun").Allowed("/x") || !AllowAll().Allowed("/x") {
		t.Error("no rules should allow")
	}
	if Parse([]byte("garbage\n\x00\xff"), "DummyRun").DeniesAll() {
		t.Error("garbage should not deny")
	}
}

func TestSiteWide(t *testing.T) {
	cases := map[string]bool{
		"User-agent: DummyRun\nDisallow: /\n":                        true,
		"User-agent: *\nDisallow: /\n":                               true,
		"User-agent: *\nDisallow: /\nUser-agent: DummyRun\nAllow: /": false,
		"User-agent: *\nDisallow: /\nAllow: /public/\n":              false,
		"User-agent: DummyRun\nDisallow: /admin\n":                   false,
		"\xef\xbb\xbfUser-agent: DummyRun\r\nDisallow: /\r\n":        true,
	}
	for file, want := range cases {
		if got := Parse([]byte(file), "DummyRun").DeniesAll(); got != want {
			t.Errorf("DeniesAll(%q) = %v, want %v", file, got, want)
		}
	}
	if !DenyAll().DeniesAll() || DenyAll().Allowed("/") || !DenyAll().Allowed("/robots.txt") {
		t.Error("DenyAll")
	}
}

func TestSizeLimit(t *testing.T) {
	big := strings.Repeat("# filler\n", MaxBytes/9+10) + "User-agent: *\nDisallow: /\n"
	if !Parse([]byte(big), "DummyRun").Allowed("/") {
		t.Error("rules past the size limit should be ignored")
	}
}

func TestFetch(t *testing.T) {
	var gotUA string
	status := http.StatusOK
	body := "User-agent: DummyRun\nDisallow: /secret\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	client := srv.Client()
	res, err := Fetch(context.Background(), client, srv.URL+"/some/page?x=1", "DummyRun", "DummyRun/9 (+https://example.test/)")
	if err != nil {
		t.Fatal(err)
	}
	if res.Rules.Allowed("/secret") || !res.Rules.Allowed("/open") || res.Origin != srv.URL || !strings.Contains(res.Status, "dummyrun") {
		t.Errorf("result %+v", res)
	}
	if gotUA != "DummyRun/9 (+https://example.test/)" {
		t.Errorf("user agent %q", gotUA)
	}
	for code, wantAllowed := range map[int]bool{404: true, 403: true, 410: true, 500: false, 503: false} {
		status, body = code, "User-agent: *\nDisallow: /\n"
		res, err = Fetch(context.Background(), client, srv.URL, "DummyRun", "")
		if err != nil {
			t.Fatal(err)
		}
		if res.Rules.Allowed("/x") != wantAllowed {
			t.Errorf("status %d: allowed %v, want %v (%s)", code, res.Rules.Allowed("/x"), wantAllowed, res.Status)
		}
	}
	srv.Close()
	res, err = Fetch(context.Background(), nil, srv.URL, "DummyRun", "")
	if err != nil || res.Rules.Allowed("/x") || !strings.Contains(res.Status, "off limits") {
		t.Errorf("unreachable: %+v %v", res, err)
	}
	if _, err := Fetch(context.Background(), nil, "not a url", "DummyRun", ""); err == nil {
		t.Error("bad URL accepted")
	}
}

func TestPercentEncodingIsNormalised(t *testing.T) {
	r := Parse([]byte("User-agent: *\nDisallow: /日本\nDisallow: /a%62c\nDisallow: /x%2fy\n"), "DummyRun")
	for p, want := range map[string]bool{
		"/%E6%97%A5%E6%9C%AC": false, "/%e6%97%a5%e6%9c%ac/x": false, "/日本": false, "/abc": false, "/a%62c": false,
		"/x%2Fy": false, "/x%2fy": false, "/x/y": true, "/本": true,
	} {
		if got := r.Allowed(p); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", p, got, want)
		}
	}
	// A "%" that does not start an escape stays as it is, even at the end of the string.
	if !Parse([]byte("User-agent: *\nDisallow: /50%\n"), "DummyRun").Allowed("/other") {
		t.Error("lone percent sign")
	}
}
