package a11y

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/links"
	"github.com/dummy-run/dummy-run/internal/pdfua"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

// RulePDFUA is a linked PDF that fails PDF/UA-1. It is advice: PDF/UA is a standard for the file, and a page can link
// to a PDF it doesn't own.
const RulePDFUA = "dummyrun-pdf-ua"

// maxPDFs is the most PDFs one sweep checks.
const maxPDFs = 10

// maxPDFBytes is the largest PDF the sweep downloads. Tests lower it.
var maxPDFBytes = 25 << 20

// findVeraPDF and runVeraPDF are pdfua's Find and Check. Tests replace them so they don't need veraPDF.
var (
	findVeraPDF = pdfua.Find
	runVeraPDF  = pdfua.Check
)

// PDFCheck is one linked PDF and what veraPDF found in it.
type PDFCheck struct {
	URL string `json:"url"`
	// Page is the first page that links to it, and Link the link there.
	Page      string       `json:"page"`
	Link      string       `json:"link"`
	Compliant bool         `json:"compliant"`
	Rules     []pdfua.Rule `json:"rules,omitempty"`
	Error     string       `json:"error,omitempty"`
	setting   string
}

// pdfLinksJS lists the shown links to PDF files on the page, resolved against the page. Called with (max).
const pdfLinksJS = `(max) => {
  const shown = (el) => {
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true })) return false;
    const r = el.getBoundingClientRect();
    return r.width > 1 && r.height > 1;
  };
  const out = [];
  for (const a of document.querySelectorAll("a[href]")) {
    if (out.length >= max) break;
    let u;
    try { u = new URL(a.getAttribute("href"), document.baseURI); } catch (e) { continue; }
    if (!/^https?:$/.test(u.protocol) || !/\.pdf$/i.test(u.pathname) || !shown(a)) continue;
    u.hash = "";
    const sel = a.id && document.querySelectorAll("#" + CSS.escape(a.id)).length === 1 ? "#" + CSS.escape(a.id) : "a[href=\"" + CSS.escape(a.getAttribute("href")) + "\"]";
    out.push({ url: u.href, selector: sel });
  }
  return out;
}`

// pdfCollect adds the page's links to PDFs on the swept sites to the sweep's list, up to maxPDFs across the sweep.
// A PDF elsewhere is left out: a scan checks the files of the site it was asked about.
func pdfCollect(ctx context.Context, conn *bidi.Conn, cx string, cfg Config, res *PageResult, rep *Report) {
	if len(rep.pdfs) >= maxPDFs {
		return
	}
	var found []struct {
		URL      string `json:"url"`
		Selector string `json:"selector"`
	}
	if err := conn.CallFunctionJSONIn(ctx, cx, "dummyrun-pdf", pdfLinksJS, &found, 50); err != nil {
		res.PDFError = firstLine(err.Error())
		return
	}
	hosts := map[string]bool{}
	for _, s := range cfg.URLs {
		if u, err := url.Parse(s); err == nil {
			hosts[strings.ToLower(u.Host)] = true
		}
	}
	for _, f := range found {
		u, err := url.Parse(f.URL)
		if err != nil || !hosts[strings.ToLower(u.Host)] || rep.pdfSeen[f.URL] || len(rep.pdfs) >= maxPDFs {
			continue
		}
		if ok, _ := rep.robotsAllows(ctx, cfg, f.URL); !ok {
			continue
		}
		if rep.pdfSeen == nil {
			rep.pdfSeen = map[string]bool{}
		}
		rep.pdfSeen[f.URL] = true
		rep.pdfs = append(rep.pdfs, PDFCheck{URL: f.URL, Page: res.URL, Link: f.Selector, setting: res.Setting()})
	}
}

// pdfCheck downloads the PDFs the sweep collected and checks them with veraPDF in one run, so Java starts once. Each
// file that fails becomes advice on the first page that links to it.
func pdfCheck(ctx context.Context, cfg Config, rep *Report) {
	if len(rep.pdfs) == 0 {
		return
	}
	bin, err := findVeraPDF()
	if err != nil {
		rep.PDFError = err.Error()
		return
	}
	dir, err := os.MkdirTemp("", "dummyrun-pdf-")
	if err != nil {
		rep.PDFError = firstLine(err.Error())
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	var files []string
	var which []int
	for i := range rep.pdfs {
		p := &rep.pdfs[i]
		name := filepath.Join(dir, fmt.Sprintf("file%d.pdf", i))
		if err := download(ctx, cfg, p.URL, name); err != nil {
			p.Error = firstLine(err.Error())
			continue
		}
		files, which = append(files, name), append(which, i)
	}
	if len(files) > 0 {
		cctx, cancel := context.WithTimeout(ctx, time.Duration(30+20*len(files))*time.Second)
		defer cancel()
		got, err := runVeraPDF(cctx, bin, files...)
		if err != nil {
			rep.PDFError = firstLine(err.Error())
			for _, i := range which {
				rep.pdfs[i].Error = "veraPDF didn't finish, so the file wasn't checked"
			}
		} else {
			for k, i := range which {
				rep.pdfs[i].Compliant, rep.pdfs[i].Rules, rep.pdfs[i].Error = got[k].Compliant, got[k].Rules, got[k].Error
			}
		}
	}
	rep.PDFs = rep.pdfs
	for _, p := range rep.pdfs {
		if p.Compliant || p.Error != "" || len(p.Rules) == 0 {
			continue
		}
		for i := range rep.Results {
			r := &rep.Results[i]
			if r.URL != p.Page || r.Setting() != p.setting {
				continue
			}
			r.Advice = append(r.Advice, pdfFinding(p, pagePath(r.URL), cfg.Allow))
			break
		}
	}
}

// pdfFinding is the advice for one PDF that fails PDF/UA-1.
func pdfFinding(p PDFCheck, path string, allow *AllowList) Finding {
	name := p.URL
	if u, err := url.Parse(p.URL); err == nil {
		name = filepath.Base(u.Path)
	}
	first := p.Rules[0]
	d := fmt.Sprintf("%s fails %d PDF/UA-1 requirements, such as %s: %s", name, len(p.Rules), first.Clause, strings.TrimSuffix(first.Description, "."))
	n := Node{Target: p.Link, Detail: d}
	if e := allow.Match(RulePDFUA, path, n.Target); e != nil {
		n.Allowed, n.Reason = true, e.Reason
	}
	return Finding{Rule: RulePDFUA, Help: "A linked PDF that doesn't meet PDF/UA-1, the standard for accessible PDF",
		HelpURL: "https://docs.verapdf.org/validation/", Nodes: []Node{n}}
}

// pdfClient follows a redirect only to the PDF's own host or to a public address. Unlike robots.txt, even a private
// origin, such as a site on localhost, can't send the download on to another private address.
func pdfClient(base *http.Client, host string) *http.Client {
	c := http.Client{Timeout: 30 * time.Second}
	if base != nil {
		c = *base
	}
	if c.Transport == nil {
		c.Transport = pdfTransport(host)
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if !strings.EqualFold(req.URL.Host, host) && links.PrivateHost(req.Context(), req.URL.Host) {
			return fmt.Errorf("redirects to a private address, %s", req.URL.Host)
		}
		return nil
	}
	return &c
}

// pdfTransport connects to the PDF's own host as usual, and to any other host only at an address that isn't private.
// The address is looked up once and the connection made to it, so a name can't pass the check and then resolve to a
// private address when the connection is made.
func pdfTransport(host string) http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	var d net.Dialer
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		h, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if own, _, _ := net.SplitHostPort(host); strings.EqualFold(h, host) || strings.EqualFold(h, own) {
			return d.DialContext(ctx, network, addr)
		}
		ips, err := pdfLookup(ctx, h)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("%s has no address", h)
		}
		for _, ip := range ips {
			if links.PrivateIP(ip.IP) {
				return nil, fmt.Errorf("redirects to a private address, %s", h)
			}
		}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	return t
}

// pdfLookup resolves a host name for pdfTransport. Tests replace it.
var pdfLookup = net.DefaultResolver.LookupIPAddr

// download fetches a PDF with a plain GET and nothing else, up to maxPDFBytes, and keeps it only when it starts like
// a PDF.
func download(ctx context.Context, cfg Config, raw, name string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(dctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", telemetry.RequestUserAgent(cfg.Identify))
	resp, err := pdfClient(cfg.HTTPClient, u.Host).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxPDFBytes)+1))
	if err != nil {
		return err
	}
	if len(b) > maxPDFBytes {
		return fmt.Errorf("the file is larger than %d MB, so it wasn't checked", maxPDFBytes>>20)
	}
	if !strings.Contains(string(b[:min(len(b), 1024)]), "%PDF-") {
		return errors.New("the file isn't a PDF")
	}
	return os.WriteFile(name, b, 0o600)
}
