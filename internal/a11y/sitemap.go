package a11y

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxSitemapBytes bounds one sitemap download.
const maxSitemapBytes = 8 << 20

type sitemapDoc struct {
	XMLName xml.Name
	URLs    []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

// ParseSitemap returns the page URLs in a sitemap, and the URLs of any child sitemaps when the
// document is a sitemap index.
func ParseSitemap(data []byte) (pages, children []string, err error) {
	var d sitemapDoc
	if err := xml.Unmarshal(data, &d); err != nil {
		return nil, nil, fmt.Errorf("sitemap: %w", err)
	}
	switch d.XMLName.Local {
	case "urlset", "sitemapindex":
	default:
		return nil, nil, fmt.Errorf("sitemap: unexpected root element %q", d.XMLName.Local)
	}
	for _, u := range d.URLs {
		if loc := strings.TrimSpace(u.Loc); loc != "" {
			pages = append(pages, loc)
		}
	}
	for _, s := range d.Sitemaps {
		if loc := strings.TrimSpace(s.Loc); loc != "" {
			children = append(children, loc)
		}
	}
	return pages, children, nil
}

// Rebase moves each URL onto the origin of base, keeping its path and query. A sitemap lists the
// production address; the sweep tests a local copy of the same pages.
func Rebase(urls []string, base string) ([]string, error) {
	b, err := url.Parse(base)
	if err != nil || b.Scheme == "" || b.Host == "" {
		return nil, fmt.Errorf("rebase: %q is not an absolute URL", base)
	}
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("rebase: %q: %w", raw, err)
		}
		u.Scheme, u.Host = b.Scheme, b.Host
		out = append(out, u.String())
	}
	return out, nil
}

// FetchSitemap downloads a sitemap, following a sitemap index one level deep, and returns the
// page URLs in order without duplicates.
func FetchSitemap(ctx context.Context, client *http.Client, loc string) ([]string, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	pages, children, err := fetchOne(ctx, client, loc)
	if err != nil {
		return nil, err
	}
	for _, c := range children {
		more, grand, err := fetchOne(ctx, client, c)
		if err != nil {
			return nil, err
		}
		if len(grand) > 0 {
			return nil, fmt.Errorf("sitemap %s: nested sitemap indexes are not followed", c)
		}
		pages = append(pages, more...)
	}
	return dedupe(pages), nil
}

func fetchOne(ctx context.Context, client *http.Client, loc string) (pages, children []string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("sitemap %s: %w", loc, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("sitemap %s: %w", loc, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("sitemap %s: %s", loc, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSitemapBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("sitemap %s: %w", loc, err)
	}
	return ParseSitemap(data)
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
