package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseSitemap(t *testing.T) {
	pages, children, err := ParseSitemap([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc> https://example.com/a/ </loc></url><url><loc>https://example.com/b/</loc><priority>0</priority></url></urlset>`))
	if err != nil || len(children) != 0 || !reflect.DeepEqual(pages, []string{"https://example.com/a/", "https://example.com/b/"}) {
		t.Fatalf("pages %v children %v err %v", pages, children, err)
	}
	_, children, err = ParseSitemap([]byte(`<sitemapindex><sitemap><loc>https://example.com/s1.xml</loc></sitemap></sitemapindex>`))
	if err != nil || len(children) != 1 {
		t.Fatalf("children %v err %v", children, err)
	}
	if _, _, err = ParseSitemap([]byte(`<html></html>`)); err == nil {
		t.Fatal("a page that is not a sitemap must be refused")
	}
	if _, _, err = ParseSitemap([]byte(`not xml`)); err == nil {
		t.Fatal("bad XML must be refused")
	}
}

func TestRebase(t *testing.T) {
	got, err := Rebase([]string{"https://dummyrun.dev/docs/x/?a=1", "https://dummyrun.dev/"}, "http://127.0.0.1:1313")
	if err != nil || !reflect.DeepEqual(got, []string{"http://127.0.0.1:1313/docs/x/?a=1", "http://127.0.0.1:1313/"}) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := Rebase([]string{"https://a/"}, "nope"); err == nil {
		t.Fatal("a base without a host must be refused")
	}
}

func TestFetchSitemapIndex(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/index.xml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `<sitemapindex><sitemap><loc>%s/one.xml</loc></sitemap><sitemap><loc>%s/two.xml</loc></sitemap></sitemapindex>`, srv.URL, srv.URL)
	})
	mux.HandleFunc("/one.xml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<urlset><url><loc>https://x/a</loc></url><url><loc>https://x/b</loc></url></urlset>`)
	})
	mux.HandleFunc("/two.xml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<urlset><url><loc>https://x/b</loc></url><url><loc>https://x/c</loc></url></urlset>`)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	got, err := FetchSitemap(context.Background(), nil, srv.URL+"/index.xml")
	if err != nil || !reflect.DeepEqual(got, []string{"https://x/a", "https://x/b", "https://x/c"}) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := FetchSitemap(context.Background(), nil, srv.URL+"/missing.xml"); err == nil {
		t.Fatal("a missing sitemap must be an error")
	}
}
