package validate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Messages as vnu 26.10.12 words them.
func TestRelevantKeepsWhatChangesAssistiveTechnology(t *testing.T) {
	keep := map[string]string{
		"Duplicate ID “a”.": "dummyrun-html-duplicate-id",
		"The element “a” with the attribute “href” must not appear as a descendant of the “button” element.":                                                       "dummyrun-html-nested-interactive",
		"The element “button” must not appear as a descendant of the “a” element.":                                                                                 "dummyrun-html-nested-interactive",
		"Bad value “btn-primary” for attribute “role” on element “div”.":                                                                                           "dummyrun-html-aria",
		"Bad value “maybe” for attribute “aria-checked” on element “span”.":                                                                                        "dummyrun-html-aria",
		"Element “div” not allowed as child of element “ul” in this context. (Suppressing further errors from this subtree.)":                                      "dummyrun-html-structure",
		"Start tag “div” seen in “table”.":                                                                                                                         "dummyrun-html-structure",
		"End tag “b” violates nesting rules.":                                                                                                                      "dummyrun-html-structure",
		"The value of the “for” attribute of the “label” element must be the ID of a non-hidden form control.":                                                     "dummyrun-html-broken-reference",
		"An “img” element must have an “alt” attribute, except under certain conditions. For details, consult guidance on providing text alternatives for images.": "dummyrun-html-img-alt",
	}
	for msg, want := range keep {
		r, ok := Relevant(Message{Type: "error", Message: msg})
		if !ok || r.ID != want {
			t.Errorf("%q: got %q %v, want %q", msg, r.ID, ok, want)
		}
	}
	for _, msg := range []string{
		"The “center” element is obsolete. Use CSS instead.",
		"Stray end tag “span”.",
		"No “p” element in scope but a “p” end tag seen.",
		"Cannot recover after last error. Any further errors will be ignored.",
	} {
		if r, ok := Relevant(Message{Type: "error", Message: msg}); ok {
			t.Errorf("%q kept as %s", msg, r.ID)
		}
	}
	if _, ok := Relevant(Message{Type: "info", SubType: "warning", Message: "Duplicate ID “a”."}); ok {
		t.Error("a warning was kept")
	}
}

func TestCheckRunsVnuOnEachDocument(t *testing.T) {
	vnu, err := Find()
	if err != nil {
		t.Skip("vnu is not installed: ", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	got, err := Check(ctx, vnu,
		`<!doctype html><html lang="en"><title>a</title><p id="x">1</p><p id="x">2</p></html>`,
		`<!doctype html><html lang="en"><title>b</title><p>fine</p></html>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want two lists, got %d", len(got))
	}
	dup := false
	for _, m := range got[0] {
		if r, ok := Relevant(m); ok && r.ID == "dummyrun-html-duplicate-id" {
			dup = true
		}
	}
	if !dup {
		t.Errorf("no duplicate ID in the first document: %+v", got[0])
	}
	for _, m := range got[1] {
		if m.Type == "error" {
			t.Errorf("the second document has an error: %+v", m)
		}
	}
}

func TestVnuVersionMatchesDockerfile(t *testing.T) {
	b, err := os.ReadFile("../../docker/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\nARG VNU_VERSION="+VnuVersion+"\n") {
		t.Errorf("docker/Dockerfile does not pin the Nu HTML Checker %s", VnuVersion)
	}
}

// vnu reports a failure to read its input with no document. That is an error, not a clean page.
func TestCheckFailsOnAnErrorWithNoDocument(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "vnu")
	script := "#!/bin/sh\necho '{\"messages\":[{\"type\":\"non-document-error\",\"subType\":\"io\",\"message\":\"Cannot read the file.\"}]}'\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil { //nolint:gosec // a test script must be executable
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := Check(ctx, fake, "<!doctype html><title>x</title>")
	if err == nil || !strings.Contains(err.Error(), "Cannot read the file.") {
		t.Fatalf("got %v, %v; want an error naming vnu's message", res, err)
	}
}

// Output vnu didn't finish, or that isn't about the files it was given, is an error too. An empty list is clean.
func TestCheckFailsOnOutputThatIsNotAboutThePages(t *testing.T) {
	for _, tc := range []struct{ name, out, want string }{
		{"no list", `{}`, "no messages list"},
		{"other file", `{"messages":[{"type":"error","url":"file:/elsewhere/page.html","message":"Stray end tag."}]}`, "wasn't given"},
		{"empty list", `{"messages":[]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := filepath.Join(t.TempDir(), "vnu")
			if err := os.WriteFile(fake, []byte("#!/bin/sh\necho '"+tc.out+"'\n"), 0o700); err != nil { //nolint:gosec // a test script must be executable
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			res, err := Check(ctx, fake, "<!doctype html><title>x</title>")
			if tc.want == "" {
				if err != nil || len(res) != 1 || len(res[0]) != 0 {
					t.Fatalf("got %v, %v; want one clean page", res, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, %v; want an error with %q", res, err, tc.want)
			}
		})
	}
}
