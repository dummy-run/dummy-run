package report

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	texttemplate "text/template" // nosemgrep: go.lang.security.audit.xss.import-text-template.import-text-template -- the Markdown report is not HTML
)

//go:embed templates/report.html.tmpl templates/report.md.tmpl
var templateFS embed.FS

// Formats lists the output formats.
var Formats = []string{"html", "md", "json", "sarif", "junit", "badge", "shields"}

// maxEmbedBytes is the largest screenshot embedded in HTML. A larger file is left out and named.
const maxEmbedBytes = 4 << 20

// Render writes the report in one format.
func Render(w io.Writer, rep *Report, format string) error {
	switch format {
	case "html":
		return renderHTML(w, rep)
	case "md":
		return renderMarkdown(w, rep)
	case "json":
		return renderJSON(w, rep)
	case "sarif":
		return renderSARIF(w, rep)
	case "badge":
		return RenderBadge(w, rep)
	case "shields":
		return RenderShields(w, rep)
	case "junit":
		return renderJUnit(w, rep)
	}
	return fmt.Errorf("unknown format %q: the formats are %s", format, strings.Join(Formats, ", "))
}

func renderJSON(w io.Writer, rep *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func cap1(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// codes writes each value as inline code separated by spaces, so a long list is not one comma-heavy sentence.
func codes(l []string) string {
	parts := make([]string, len(l))
	for i, s := range l {
		parts[i] = "`" + s + "`"
	}
	return strings.Join(parts, " ")
}

func f1(p *float64) string {
	if p == nil {
		return "not estimable"
	}
	return fmt.Sprintf("%.1f", *p)
}

func f2(p *float64) string {
	if p == nil {
		return "not estimable"
	}
	return fmt.Sprintf("%.2f", *p)
}

func stepText(s StepRef) string {
	out := cap1(s.Action)
	if s.Target != "" {
		out += " " + s.Target
	}
	if s.Route != "" {
		out += " on " + s.Route
	} else if s.URL != "" {
		out += " at " + s.URL
	}
	return out + "."
}

func statusText(s string) string {
	if s == StatusConfirmed {
		return "Confirmed by a reviewer"
	}
	return "Not reviewed"
}

func baselineText(s string) string {
	switch s {
	case BaselineNew:
		return "New since the baseline"
	case BaselineStillPresent:
		return "Still present"
	case BaselineFixed:
		return "Fixed"
	}
	return s
}

func commonFuncs() map[string]any {
	return map[string]any{
		"cap":            cap1,
		"join":           func(l []string) string { return strings.Join(l, ", ") },
		"usd":            func(v float64) string { return fmt.Sprintf("$%.2f", v) },
		"codes":          codes,
		"f1":             f1,
		"f2":             f2,
		"stepText":       stepText,
		"statusText":     statusText,
		"confidenceText": confidenceText,
		"baselineText":   baselineText,
		"severities":     func() []string { return Severities },
		"sevGroups":      sevGroups,
		"segments":       segments,
		"longDate":       longDate,
		"period":         period,
		"howSure":        howSure,
		"impactSev":      impactSeverity,
		"plural":         plural,
		"pages":          pagesTested,
		"upper":          upperFirst,
		"cell":           mdCell,
		"inline":         mdInline,
		"inc":            func(i int) int { return i + 1 },
		"fixRules":       fixRules,
		"colourRules":    colourRules,
		"symptomText":    symptomText,
		"placeText":      placeText,
		"whoText":        whoText,
		"reproSteps":     reproSteps,
		"shotCaption":    shotCaption,
		"lookAt":         lookAt,
		"lead":           lead,
	}
}

func parseHTML() (*htmltemplate.Template, error) {
	funcs := htmltemplate.FuncMap(commonFuncs())
	funcs["shot"] = embedShot
	funcs["asset"] = asset
	return htmltemplate.New("report.html.tmpl").Funcs(funcs).ParseFS(templateFS, "templates/report.html.tmpl")
}

func renderHTML(w io.Writer, rep *Report) error {
	t, err := parseHTML()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, rep); err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}

var imageTypes = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}

// embedShot returns a data URI for a screenshot, or "" when the file is missing or too large.
func embedShot(s Screenshot) htmltemplate.URL {
	if s.File == "" {
		return ""
	}
	fi, err := os.Stat(s.File)
	if err != nil || fi.Size() > maxEmbedBytes {
		return ""
	}
	data, err := os.ReadFile(s.File)
	if err != nil {
		return ""
	}
	typ, ok := imageTypes[strings.ToLower(filepath.Ext(s.File))]
	if !ok {
		return ""
	}
	// The type comes from a fixed list and the rest is base64, so nothing in the URL is markup.
	// nosemgrep: go.lang.security.audit.xss.template-html-does-not-escape.unsafe-template-type, go.lang.security.audit.net.unescaped-data-in-url.unescaped-data-in-url
	return htmltemplate.URL("data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(data)) //nolint:gosec // see above
}

func renderMarkdown(w io.Writer, rep *Report) error {
	t, err := texttemplate.New("report.md.tmpl").Funcs(texttemplate.FuncMap(commonFuncs())).ParseFS(templateFS, "templates/report.md.tmpl")
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, rep); err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	return err
}

// mdCell makes free text safe inside a Markdown table cell: a pipe would start a new column and a
// newline would end the row.
func mdCell(v any) string {
	s := fmt.Sprint(v)
	s = strings.NewReplacer("|", `\|`, "\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return s
}

// mdInline makes free text safe inside a Markdown code span or a paragraph: a backtick would end the
// span and a newline would split the paragraph.
func mdInline(v any) string {
	s := fmt.Sprint(v)
	return strings.NewReplacer("`", "'", "\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

// colourRules keeps the failed rules that have colour suggestions, in report order.
func colourRules(rules []AxeRule) []AxeRule {
	var out []AxeRule
	for _, r := range rules {
		if len(r.Colours) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// fixRules keeps the failed rules the fix library has an entry for, in report order.
func fixRules(rules []AxeRule) []AxeRule {
	var out []AxeRule
	for _, r := range rules {
		if r.Fix != nil {
			out = append(out, r)
		}
	}
	return out
}
