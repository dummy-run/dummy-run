package a11y

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/validate"
)

// htmlMaxDoc is the most of one document the HTML check reads. Past it the document is cut and the result says so.
const htmlMaxDoc = 2 << 20

// htmlDocsJS returns the page's HTML twice: as the server sent it, fetched again from the page with the page's own
// cookies, and as the browser built it, with what scripts added. Parse errors are only in the first, and errors in
// content scripts add are only in the second. A page whose source can't be fetched again gives "" for it.
const htmlDocsJS = `async (max) => {
  let served = "";
  try {
    const r = await fetch(location.href, { credentials: "same-origin", cache: "force-cache", redirect: "follow" });
    if (r.ok && /html/i.test(r.headers.get("content-type") || "")) served = await r.text();
  } catch (e) {
    served = "";
  }
  const dt = document.doctype ? "<!DOCTYPE " + document.doctype.name + ">\n" : "";
  const built = dt + document.documentElement.outerHTML;
  return { served: served.slice(0, max), built: built.slice(0, max), cut: served.length > max || built.length > max };
}`

// htmlFindings turns vnu's accessibility-relevant errors into advice, one finding per rule. An error both copies of
// the page have counts once, and an error a copy has twice, such as two images without alt text, counts twice.
// stopped says vnu hit a fatal error in a copy and read no further.
func htmlFindings(served, built []validate.Message, path string, allow *AllowList) (
	out []Finding, ignored int, stopped bool,
) {
	by := map[string]*Finding{}
	inServed := map[string]int{}
	inBuilt := map[string]int{}
	add := func(ms []validate.Message, from string) {
		for _, m := range ms {
			if m.SubType == "fatal" {
				stopped = true
			}
			r, ok := validate.Relevant(m)
			if !ok {
				if m.Type == "error" && m.SubType != "fatal" {
					ignored++
				}
				continue
			}
			key := r.ID + "\x00" + m.Message
			if from == "served" {
				inServed[key]++
			} else if inBuilt[key]++; inBuilt[key] <= inServed[key] {
				continue
			}
			f, ok := by[r.ID]
			if !ok {
				f = &Finding{Rule: r.ID, Help: r.Help, WCAG: []string{r.WCAG}, HelpURL: "https://validator.w3.org/nu/about.html", Nodes: []Node{}}
				by[r.ID] = f
			}
			detail := m.Message
			if from == "served" && m.LastLine > 0 {
				detail = fmt.Sprintf("line %d of the source: %s", m.LastLine, m.Message)
			}
			if x := strings.TrimSpace(m.Extract); x != "" {
				detail += " In: " + squashSpace(x)
			}
			n := Node{Target: "html", Detail: detail}
			if e := allow.Match(r.ID, path, n.Target); e != nil {
				n.Allowed, n.Reason = true, e.Reason
			}
			f.Nodes = append(f.Nodes, n)
		}
	}
	add(served, "served")
	add(built, "built")
	for _, r := range validate.Rules {
		if f, ok := by[r.ID]; ok {
			out = append(out, *f)
		}
	}
	return out, ignored, stopped
}

// cutBytes keeps at most n bytes of s, ending on a whole character, and says whether it cut any.
func cutBytes(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n], true
}

func squashSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// htmlCheck runs vnu on the page. A missing vnu or a failed check is recorded on the result and does not stop the
// page's other checks.
func htmlCheck(ctx context.Context, conn *bidi.Conn, cx string, res *PageResult, path string, allow *AllowList) {
	vnu, err := validate.Find()
	if err != nil {
		res.HTMLError = err.Error()
		return
	}
	var docs struct {
		Served string `json:"served"`
		Built  string `json:"built"`
		Cut    bool   `json:"cut"`
	}
	if err := conn.CallFunctionJSON(ctx, cx, htmlDocsJS, &docs, htmlMaxDoc); err != nil {
		res.HTMLError = firstLine(err.Error())
		return
	}
	// The page cuts each copy by characters, and vnu reads bytes, so each is cut again here to htmlMaxDoc bytes.
	var cutBuilt, cutServed bool
	docs.Built, cutBuilt = cutBytes(docs.Built, htmlMaxDoc)
	docs.Served, cutServed = cutBytes(docs.Served, htmlMaxDoc)
	docs.Cut = docs.Cut || cutBuilt || cutServed
	in := []string{docs.Built}
	if docs.Served != "" {
		in = append(in, docs.Served)
	}
	got, err := validate.Check(ctx, vnu, in...)
	if err != nil {
		res.HTMLError = firstLine(err.Error())
		return
	}
	var served []validate.Message
	if len(got) > 1 {
		served = got[1]
	}
	found, ignored, stopped := htmlFindings(served, got[0], path, allow)
	res.HTMLChecked, res.HTMLSourceChecked, res.HTMLPartial, res.HTMLIgnored = true, docs.Served != "", docs.Cut, ignored
	res.HTMLStopped = stopped
	res.Advice = append(res.Advice, found...)
}
