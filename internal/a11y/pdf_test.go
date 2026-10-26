package a11y

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/pdfua"
)

func TestPDFsAreCheckedOnceAndOnlyOnTheSweptSite(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	if _, err := pdfua.Find(); err != nil {
		t.Skip("set DUMMYRUN_VERAPDF or put verapdf on the PATH to run the PDF check")
	}
	untagged, err := os.ReadFile("../pdfua/testdata/untagged.pdf")
	if err != nil {
		t.Fatal(err)
	}
	var elsewhereHit atomic.Bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhereHit.Store(true)
		_, _ = w.Write(untagged)
	}))
	defer elsewhere.Close()
	var gets, posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			if r.Method != http.MethodGet {
				posts.Add(1)
			}
			gets.Add(1)
		}
		switch r.URL.Path {
		case "/files/report.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(untagged)
		case "/files/notes.pdf":
			_, _ = fmt.Fprint(w, "<html>not a pdf</html>")
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Docs</title></head><body><main><h1>Docs</h1>
<a id="r" href="/files/report.pdf">Annual report (PDF)</a> <a href="/files/report.pdf#page=2">Page 2</a>
<a href="/files/notes.pdf">Notes (PDF)</a> <a href="%s/other.pdf">Partner report (PDF)</a>
<div hidden><a href="/files/hidden.pdf">Hidden (PDF)</a></div></main></body></html>`, elsewhere.URL)
		}
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			gets.Store(0)
			posts.Store(0)
			elsewhereHit.Store(false)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/", srv.URL + "/again"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, PDF: true})
			if err != nil {
				t.Fatal(err)
			}
			if rep.PDFError != "" || len(rep.PDFs) != 2 {
				t.Fatalf("pdf error %q, pdfs %+v", rep.PDFError, rep.PDFs)
			}
			if gets.Load() != 2 || posts.Load() != 0 || elsewhereHit.Load() {
				t.Errorf("%d GETs (want one for each PDF), %d other requests, another host fetched %v", gets.Load(), posts.Load(), elsewhereHit.Load())
			}
			byURL := map[string]PDFCheck{}
			for _, p := range rep.PDFs {
				byURL[strings.TrimPrefix(p.URL, srv.URL)] = p
			}
			report, notes := byURL["/files/report.pdf"], byURL["/files/notes.pdf"]
			if report.Compliant || len(report.Rules) == 0 || report.Page != srv.URL+"/" || report.Link != "#r" {
				t.Errorf("report.pdf: %+v", report)
			}
			if notes.Error == "" {
				t.Errorf("notes.pdf isn't a PDF but has no error: %+v", notes)
			}
			var advice []Node
			for _, r := range rep.Results {
				for _, f := range r.Advice {
					if f.Rule == RulePDFUA {
						advice = append(advice, f.Nodes...)
					}
				}
				for _, f := range r.Violations {
					if f.Rule == RulePDFUA {
						t.Error("a PDF/UA failure is advice, not a failure")
					}
				}
			}
			if len(advice) != 1 || advice[0].Target != "#r" || !strings.Contains(advice[0].Detail, "report.pdf fails") {
				t.Errorf("advice %+v", advice)
			}
		})
	}
}

// A PDF download follows a redirect on its own host, but never one to another private address, even from a private
// origin such as localhost.
func TestPDFDownloadRefusesPrivateRedirects(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "%PDF-1.7 other")
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/moved.pdf":
			http.Redirect(w, r, "/file.pdf", http.StatusFound)
		case "/away.pdf":
			http.Redirect(w, r, other.URL+"/file.pdf", http.StatusFound)
		default:
			_, _ = fmt.Fprint(w, "%PDF-1.7 here")
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	if err := download(context.Background(), Config{}, srv.URL+"/moved.pdf", dir+"/a.pdf"); err != nil {
		t.Errorf("a redirect on the same host: %v", err)
	}
	err := download(context.Background(), Config{}, srv.URL+"/away.pdf", dir+"/b.pdf")
	if err == nil || !strings.Contains(err.Error(), "private address") {
		t.Errorf("a redirect to another private address: %v", err)
	}
	if _, err := os.Stat(dir + "/b.pdf"); err == nil {
		t.Error("the refused file was written")
	}
}

func TestSummaryListsPDFProblems(t *testing.T) {
	r := &Report{PDFs: []PDFCheck{{URL: "https://example.com/a.pdf", Rules: []pdfua.Rule{{Clause: "7.1"}}}, {URL: "https://example.com/b.pdf", Error: "veraPDF didn't finish, so the file wasn't checked"}},
		Results: []PageResult{{URL: "https://example.com/", Viewport: "desktop", Scheme: "light", PDFError: "script error"}}}
	var b strings.Builder
	r.WriteSummary(&b)
	for _, want := range []string{"PDF https://example.com/a.pdf: fails 1 PDF/UA-1", "PDF https://example.com/b.pdf: not checked: veraPDF didn't finish", "PDF https://example.com/ (", "links not read: script error"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, b.String())
		}
	}
}

func TestPDFDownloadFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gone.pdf":
			http.NotFound(w, r)
		case "/loop.pdf":
			http.Redirect(w, r, "/loop.pdf", http.StatusFound)
		default:
			_, _ = fmt.Fprint(w, "%PDF-1.7 "+strings.Repeat("x", 100))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	saved := maxPDFBytes
	maxPDFBytes = 50
	defer func() { maxPDFBytes = saved }()
	for path, want := range map[string]string{"/gone.pdf": "answered 404", "/loop.pdf": "stopped after 10 redirects", "/big.pdf": "larger than"} {
		if err := download(context.Background(), Config{}, srv.URL+path, dir+"/f.pdf"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want %q, got %v", path, want, err)
		}
	}
}

// A name that passes the check when the redirect is followed, but resolves to a private address when the
// connection is made, is refused at the connection.
func TestPDFRedirectIsCheckedWhenItConnects(t *testing.T) {
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "%PDF-1.7") }))
	defer inner.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(inner.URL, "http://"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://rebind.invalid:"+port+"/f.pdf", http.StatusFound)
	}))
	defer srv.Close()
	saved := pdfLookup
	// The name passes the redirect check at a public address, then resolves to a private one when the connection is
	// made, as a rebinding name does.
	lookups := 0
	pdfLookup = func(context.Context, string) ([]net.IPAddr, error) {
		lookups++
		if lookups == 1 {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	defer func() { pdfLookup = saved }()
	err := download(context.Background(), Config{}, srv.URL+"/away.pdf", t.TempDir()+"/f.pdf")
	if err == nil || !strings.Contains(err.Error(), "private address") || lookups != 2 {
		t.Errorf("want a private address refusal when it connects, got %v after %d lookups", err, lookups)
	}
}

// The transport tries each checked address in turn, as the standard dialer does, uses no proxy, and stays in place
// whatever transport the sweep's client has.
func TestPDFTransportFallsBackAndIgnoresProxyAndBaseTransport(t *testing.T) {
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "%PDF-1.7") }))
	defer inner.Close()
	innerAddr := strings.TrimPrefix(inner.URL, "http://")
	_, port, _ := net.SplitHostPort(innerAddr)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://cdn.example:"+port+"/f.pdf", http.StatusFound)
	}))
	defer srv.Close()
	savedLookup, savedDial := pdfLookup, pdfDial
	defer func() { pdfLookup, pdfDial = savedLookup, savedDial }()
	pdfLookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("2001:4860:4860::8888")}, {IP: net.ParseIP("93.184.216.34")}}, nil
	}
	var tried []string
	pdfDial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		tried = append(tried, addr)
		switch {
		case strings.HasPrefix(addr, "[2001:"):
			return nil, errors.New("network is unreachable")
		case strings.HasPrefix(addr, "93.184.216.34:"):
			addr = innerAddr
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	base := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("the base transport was used")
	})}
	if err := download(context.Background(), Config{HTTPClient: base}, srv.URL+"/away.pdf", t.TempDir()+"/f.pdf"); err != nil {
		t.Fatalf("download: %v (tried %v)", err, tried)
	}
	if len(tried) != 3 || !strings.HasPrefix(tried[1], "[2001:") || !strings.HasPrefix(tried[2], "93.184.216.34:") {
		t.Errorf("tried %v, want the PDF's host, then the IPv6 address, then the IPv4 one", tried)
	}
	if tr, ok := pdfTransport("example.com").(*http.Transport); !ok || tr.Proxy != nil {
		t.Error("the PDF transport uses a proxy")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// When veraPDF fails, every PDF that downloaded says it wasn't checked, and the sweep says why.
func TestPDFCheckWhenVeraPDFFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "%PDF-1.7") }))
	defer srv.Close()
	savedFind, savedRun := findVeraPDF, runVeraPDF
	findVeraPDF = func() (string, error) { return "verapdf", nil }
	runVeraPDF = func(context.Context, string, ...string) ([]pdfua.Result, error) {
		return nil, errors.New("java crashed")
	}
	defer func() { findVeraPDF, runVeraPDF = savedFind, savedRun }()
	rep := &Report{pdfs: []PDFCheck{{URL: srv.URL + "/a.pdf"}, {URL: srv.URL + "/b.pdf"}}}
	pdfCheck(context.Background(), Config{}, rep)
	if !strings.Contains(rep.PDFError, "java crashed") || len(rep.PDFs) != 2 {
		t.Fatalf("pdf error %q, pdfs %+v", rep.PDFError, rep.PDFs)
	}
	for _, p := range rep.PDFs {
		if p.Error == "" || p.Compliant {
			t.Errorf("%s has no error: %+v", p.URL, p)
		}
	}
}

func TestOwnAddr(t *testing.T) {
	for _, c := range []struct {
		addr, host string
		want       bool
	}{
		{"example.com:443", "example.com", true},
		{"example.com:80", "example.com", true},
		{"example.com:8080", "example.com", false},
		{"127.0.0.1:5000", "127.0.0.1:5000", true},
		{"127.0.0.1:5001", "127.0.0.1:5000", false},
		{"[::1]:443", "[::1]", true},
		{"cdn.example:443", "example.com", false},
	} {
		if got := ownAddr(c.addr, c.host); got != c.want {
			t.Errorf("ownAddr(%q, %q) = %v", c.addr, c.host, got)
		}
	}
}
