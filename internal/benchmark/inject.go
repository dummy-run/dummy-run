// Package benchmark plants known barriers in copies of pages and scores how many of them a sweep finds.
//
// Each Defect changes a clean page in one way a disabled visitor would meet, such as a form field with no label or a
// page that traps the keyboard. The sweep runs on the clean page and on the changed copy, and a defect counts as found
// when the copy shows one of the defect's rules that the clean page did not. The score is recall per check.
//
// The injector is written for this project. It borrows the idea of planting barriers from published work on
// accessibility benchmarks, and none of its code.
package benchmark

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Defect is one barrier the injector can plant.
type Defect struct {
	// ID names the defect, such as "remove-label".
	ID string
	// Describe says what the defect does to the page, in a sentence.
	Describe string
	// Rules are the finding rule IDs that count as finding the defect. Any one is enough.
	Rules []string
	// Keyboard and Markup say which optional checks the sweep needs to find it.
	Keyboard, Markup bool
	// apply changes the parsed page and reports whether it found something to change.
	apply func(doc *html.Node) bool
}

// Plant returns src with the defect planted, and false when the page has nothing the defect can change.
func (d Defect) Plant(src string) (string, bool) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil || !d.apply(doc) {
		return "", false
	}
	var sb strings.Builder
	if err := html.Render(&sb, doc); err != nil {
		return "", false
	}
	return sb.String(), true
}

// Defects are the barriers the injector knows, in the order the scorer lists them.
var Defects = []Defect{
	{ID: "remove-label", Describe: "Delete the visible labels of the form fields", Rules: []string{"label"},
		apply: func(doc *html.Node) bool { return removeAll(doc, hasTag(atom.Label)) }},
	{ID: "remove-alt", Describe: "Delete the text alternative of the first image", Rules: []string{"image-alt"},
		apply: func(doc *html.Node) bool {
			n := find(doc, func(n *html.Node) bool { return isTag(n, atom.Img) && hasAttr(n, "alt") })
			return n != nil && delAttr(n, "alt")
		}},
	{ID: "remove-lang", Describe: "Delete the page language", Rules: []string{"html-has-lang"},
		apply: func(doc *html.Node) bool {
			n := find(doc, hasTag(atom.Html))
			return n != nil && delAttr(n, "lang")
		}},
	{ID: "remove-title", Describe: "Delete the page title", Rules: []string{"document-title"},
		apply: func(doc *html.Node) bool { return removeAll(doc, hasTag(atom.Title)) }},
	{ID: "empty-button", Describe: "Empty the first button of its words and its name", Rules: []string{"button-name"},
		apply: func(doc *html.Node) bool {
			n := find(doc, hasTag(atom.Button))
			if n == nil {
				return false
			}
			empty(n)
			return true
		}},
	{ID: "empty-link", Describe: "Empty the first link in the main content of its words and its name", Rules: []string{"link-name"},
		apply: func(doc *html.Node) bool {
			m := find(doc, hasTag(atom.Main))
			if m == nil {
				return false
			}
			n := find(m, hasTag(atom.A))
			if n == nil {
				return false
			}
			empty(n)
			return true
		}},
	{ID: "pale-text", Describe: "Turn the first paragraph pale grey on white", Rules: []string{"color-contrast"},
		apply: func(doc *html.Node) bool {
			n := find(doc, hasTag(atom.P))
			if n == nil {
				return false
			}
			setAttr(n, "style", "color:#c8c8c8;background:#fff")
			return true
		}},
	{ID: "skip-heading", Describe: "Turn the first level 2 heading into a level 4", Rules: []string{"heading-order"},
		apply: func(doc *html.Node) bool {
			n := find(doc, hasTag(atom.H2))
			if n == nil {
				return false
			}
			n.DataAtom, n.Data = atom.H4, "h4"
			return true
		}},
	{ID: "block-zoom", Describe: "Stop pinch zoom in the viewport tag", Rules: []string{"meta-viewport"},
		apply: func(doc *html.Node) bool {
			n := find(doc, func(n *html.Node) bool { return isTag(n, atom.Meta) && attr(n, "name") == "viewport" })
			if n == nil {
				return false
			}
			setAttr(n, "content", "width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no")
			return true
		}},
	{ID: "no-main", Describe: "Turn the main landmark into a plain box", Rules: []string{"landmark-one-main"},
		apply: func(doc *html.Node) bool {
			n := find(doc, hasTag(atom.Main))
			if n == nil {
				return false
			}
			n.DataAtom, n.Data = atom.Div, "div"
			return true
		}},
	{ID: "hide-focusable", Describe: "Hide the main content from assistive technology while its controls can take focus",
		Rules: []string{"aria-hidden-focus"},
		apply: func(doc *html.Node) bool {
			n := find(doc, hasTag(atom.Main))
			if n == nil {
				return false
			}
			setAttr(n, "aria-hidden", "true")
			return true
		}},
	{ID: "remove-outline", Describe: "Remove the focus outline from every control", Keyboard: true,
		Rules: []string{"dummyrun-focus-not-visible"},
		apply: func(doc *html.Node) bool {
			return addToHead(doc, styleNode(`*:focus,*:focus-visible{outline:none!important;box-shadow:none!important;border-color:inherit!important}`))
		}},
	{ID: "keyboard-trap", Describe: "Cycle focus between the first two controls in the main content, so Tab never leaves them",
		Keyboard: true, Rules: []string{"dummyrun-keyboard-trap"},
		apply: func(doc *html.Node) bool {
			return addToBody(doc, scriptNode(`(function(){`+
				`var c=[].slice.call(document.querySelectorAll("main a[href],main button,main input,main select"));`+
				`if(c.length<2)return;document.addEventListener("keydown",function(e){`+
				`if(e.key!=="Tab"||c.indexOf(document.activeElement)<0)return;e.preventDefault();`+
				`(document.activeElement===c[0]?c[1]:c[0]).focus()});c[0].focus()})()`))
		}},
	{ID: "break-skip-link", Describe: "Point the skip link at an id that does not exist", Keyboard: true,
		Rules: []string{"dummyrun-skip-link-broken"},
		apply: func(doc *html.Node) bool {
			n := find(doc, func(n *html.Node) bool { return isTag(n, atom.A) && hasClass(n, "skip") })
			if n == nil {
				return false
			}
			setAttr(n, "href", "#nowhere")
			return true
		}},
	{ID: "name-without-words", Describe: "Give the first button an accessible name that leaves out its visible words", Markup: true,
		Rules: []string{"dummyrun-label-in-name"},
		apply: func(doc *html.Node) bool {
			n := find(doc, func(n *html.Node) bool { return isTag(n, atom.Button) && hasAttr(n, "aria-label") })
			if n == nil {
				return false
			}
			setAttr(n, "aria-label", "Continue")
			return true
		}},
	{ID: "remove-autocomplete", Describe: "Delete the autocomplete value of the email field", Markup: true,
		Rules: []string{"dummyrun-autocomplete-missing"},
		apply: func(doc *html.Node) bool {
			n := find(doc, func(n *html.Node) bool { return isTag(n, atom.Input) && attr(n, "autocomplete") == "email" })
			return n != nil && delAttr(n, "autocomplete")
		}},
}

// Lookup returns the defect with the ID.
func Lookup(id string) (Defect, bool) {
	for _, d := range Defects {
		if d.ID == id {
			return d, true
		}
	}
	return Defect{}, false
}

func isTag(n *html.Node, a atom.Atom) bool { return n.Type == html.ElementNode && n.DataAtom == a }

func hasTag(a atom.Atom) func(*html.Node) bool { return func(n *html.Node) bool { return isTag(n, a) } }

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func setAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func delAttr(n *html.Node, key string) bool {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return true
		}
	}
	return false
}

// find returns the first node under root, in document order, that the test accepts.
func find(root *html.Node, test func(*html.Node) bool) *html.Node {
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if test(c) {
			return c
		}
		if n := find(c, test); n != nil {
			return n
		}
	}
	return nil
}

// removeAll detaches every node the test accepts and reports whether there was one.
func removeAll(root *html.Node, test func(*html.Node) bool) bool {
	var hit []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if test(c) {
				hit = append(hit, c)
				continue
			}
			walk(c)
		}
	}
	walk(root)
	for _, n := range hit {
		n.Parent.RemoveChild(n)
	}
	return len(hit) > 0
}

// empty removes a control's content and the attributes that name it.
func empty(n *html.Node) {
	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}
	for _, k := range []string{"aria-label", "aria-labelledby", "title"} {
		delAttr(n, k)
	}
}

func textElement(a atom.Atom, text string) *html.Node {
	n := &html.Node{Type: html.ElementNode, DataAtom: a, Data: a.String()}
	n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
	return n
}

func styleNode(css string) *html.Node   { return textElement(atom.Style, css) }
func scriptNode(code string) *html.Node { return textElement(atom.Script, code) }

func addToHead(doc *html.Node, n *html.Node) bool {
	h := find(doc, hasTag(atom.Head))
	if h == nil {
		return false
	}
	h.AppendChild(n)
	return true
}

func addToBody(doc *html.Node, n *html.Node) bool {
	b := find(doc, hasTag(atom.Body))
	if b == nil {
		return false
	}
	b.AppendChild(n)
	return true
}
