package page

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// In Chrome, roles, names, states and exposure come from the browser's own accessibility tree, read with
// Accessibility.getFullAXTree over the Chrome DevTools Protocol. WebDriver BiDi has no command for it, so the engine
// uses chromedriver's goog:cdp passthrough. Firefox has no passthrough and keeps names.js.
//
// Each element the tree gives a role is marked data-dummyrun-ax with its backend node ID, and the table of what
// Chrome says about those IDs is put in the engine's sandbox as __drAX, where names.js reads it. An element with no
// mark, such as one in a frame or one added after the tree was read, is named by names.js as before.

const (
	axAttr = "data-dummyrun-ax"
	// maxAXMarks caps the elements marked on one read, so a huge page costs a bounded number of commands.
	maxAXMarks = 3000
	// axWorkers is how many marking commands are in flight at once.
	axWorkers = 16
	// NamesFromChrome is Snapshot.NamesFrom when Chrome's accessibility tree named the page.
	NamesFromChrome = "chrome"
)

// axState is what the engine keeps between reads for one tab: the CDP session, and which nodes of the current
// document are already marked, so a second look marks only what is new.
type axState struct {
	mu      sync.Mutex
	off     bool
	session string
	root    int
	marked  map[int]bool
}

// axEntry is one element as names.js reads it. R is nil when Chrome's role is not an ARIA role names.js knows,
// and names.js then works the role out itself.
type axEntry struct {
	R *string  `json:"r,omitempty"`
	N string   `json:"n"`
	I bool     `json:"i,omitempty"`
	S []string `json:"s,omitempty"`
}

type axValue struct {
	Value json.RawMessage `json:"value"`
}

type axNode struct {
	Ignored    bool    `json:"ignored"`
	Role       axValue `json:"role"`
	Name       axValue `json:"name"`
	Properties []struct {
		Name  string  `json:"name"`
		Value axValue `json:"value"`
	} `json:"properties"`
	BackendDOMNodeID int `json:"backendDOMNodeId"`
}

// ariaRoles are the roles names.js uses as they are. Chrome's own roles, such as DisclosureTriangle or date, are
// left for names.js to map.
var ariaRoles = map[string]bool{}

func init() {
	for _, r := range []string{
		"alert", "alertdialog", "application", "article", "banner", "blockquote", "button", "caption", "cell",
		"checkbox", "code", "columnheader", "combobox", "complementary", "contentinfo", "definition", "deletion",
		"dialog", "document", "emphasis", "feed", "figure", "form", "grid", "gridcell", "group", "heading", "img",
		"insertion", "link", "list", "listbox", "listitem", "log", "main", "mark", "marquee", "math", "menu",
		"menubar", "menuitem", "menuitemcheckbox", "menuitemradio", "meter", "navigation", "note", "option",
		"paragraph", "progressbar", "radio", "radiogroup", "region", "row", "rowgroup", "rowheader", "scrollbar",
		"search", "searchbox", "separator", "slider", "spinbutton", "status", "strong", "subscript", "superscript",
		"switch", "tab", "table", "tablist", "tabpanel", "term", "textbox", "timer", "toolbar", "tooltip",
		"tree", "treegrid", "treeitem",
	} {
		ariaRoles[r] = true
	}
}

func rawString(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return string(v)
}

// chromeRole maps Chrome's role to the role names.js would give: "image" is img, and generic or none is no role.
// ok is false for a role names.js should work out itself.
func chromeRole(r string) (role string, ok bool) {
	switch r {
	case "image":
		return "img", true
	case "generic", "none", "presentation":
		return "", true
	case "StaticText", "InlineTextBox", "LineBreak", "ListMarker", "RootWebArea":
		// Text and the document itself, which names.js never asks about.
		return "", true
	}
	return r, ariaRoles[r]
}

// axStates gives the states names.js reports, in its order, from Chrome's properties. aria-current is not one of
// Chrome's properties, so names.js adds it.
func axStates(n axNode) []string {
	var disabled, checked, required, invalid bool
	expanded := ""
	for _, p := range n.Properties {
		v := rawString(p.Value.Value)
		switch p.Name {
		case "disabled":
			disabled = v == "true"
		case "checked":
			checked = v == "true"
		case "expanded":
			switch v {
			case "true":
				expanded = "expanded"
			case "false":
				expanded = "collapsed"
			}
		case "required":
			required = v == "true"
		case "invalid":
			invalid = v != "" && v != "false"
		}
	}
	var out []string
	if disabled {
		out = append(out, "disabled")
	}
	if checked {
		out = append(out, "checked")
	}
	if expanded != "" {
		out = append(out, expanded)
	}
	if required {
		out = append(out, "required")
	}
	if invalid {
		out = append(out, "invalid")
	}
	return out
}

// axTable picks the nodes to mark from a tree, those with a role names.js would list, up to maxAXMarks.
func axTable(nodes []axNode) map[int]axEntry {
	out := map[int]axEntry{}
	for _, n := range nodes {
		if n.BackendDOMNodeID <= 0 || len(out) >= maxAXMarks {
			continue
		}
		role, ok := chromeRole(rawString(n.Role.Value))
		if role == "" {
			continue
		}
		e := axEntry{N: rawString(n.Name.Value), I: n.Ignored, S: axStates(n)}
		if ok {
			e.R = &role
		}
		out[n.BackendDOMNodeID] = e
	}
	return out
}

// readChromeAX reads Chrome's accessibility tree for the tab, marks the elements it names and puts the table in
// the sandbox. It reports whether the table is there. Outside Chrome, or when any step fails, the sandbox gets no
// table and names.js works alone.
func (s *axState) read(ctx context.Context, conn *bidi.Conn, cx string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ok := s.readLocked(ctx, conn, cx)
	if !ok {
		_ = conn.CallFunctionJSONIn(ctx, cx, sandbox, "() => { globalThis.__drAX = null; }", nil)
	}
	return ok
}

func (s *axState) readLocked(ctx context.Context, conn *bidi.Conn, cx string) bool {
	if s.off {
		return false
	}
	if s.session == "" {
		session, err := conn.CDPSession(ctx, cx)
		if err != nil {
			s.off = true
			return false
		}
		s.session = session
	}
	var doc struct {
		Root struct {
			BackendNodeID int `json:"backendNodeId"`
		} `json:"root"`
	}
	if err := conn.CDP(ctx, s.session, "DOM.getDocument", map[string]any{"depth": 0}, &doc); err != nil {
		return false
	}
	if doc.Root.BackendNodeID != s.root || s.marked == nil {
		s.root, s.marked = doc.Root.BackendNodeID, map[int]bool{}
	}
	var tree struct {
		Nodes []axNode `json:"nodes"`
	}
	if err := conn.CDP(ctx, s.session, "Accessibility.getFullAXTree", nil, &tree); err != nil {
		return false
	}
	table := axTable(tree.Nodes)
	var fresh []int
	for id := range table {
		if !s.marked[id] {
			fresh = append(fresh, id)
		}
	}
	if len(fresh) > 0 {
		var pushed struct {
			NodeIDs []int `json:"nodeIds"`
		}
		if err := conn.CDP(ctx, s.session, "DOM.pushNodesByBackendIdsToFrontend", map[string]any{"backendNodeIds": fresh}, &pushed); err != nil {
			return false
		}
		s.mark(ctx, conn, fresh, pushed.NodeIDs)
	}
	body, err := json.Marshal(table)
	if err != nil {
		return false
	}
	return conn.CallFunctionJSONIn(ctx, cx, sandbox, "(t) => { globalThis.__drAX = JSON.parse(t); }", nil, string(body)) == nil
}

// mark sets data-dummyrun-ax on each pushed node. A node that has gone, or is not an element, is skipped and
// names.js names it.
func (s *axState) mark(ctx context.Context, conn *bidi.Conn, backend, nodes []int) {
	type job struct{ backend, node int }
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for range axWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				err := conn.CDP(ctx, s.session, "DOM.setAttributeValue", map[string]any{
					"nodeId": j.node, "name": axAttr, "value": strconv.Itoa(j.backend),
				}, nil)
				if err == nil {
					mu.Lock()
					s.marked[j.backend] = true
					mu.Unlock()
				}
			}
		}()
	}
	for i, id := range backend {
		if i < len(nodes) && nodes[i] > 0 {
			jobs <- job{id, nodes[i]}
		}
	}
	close(jobs)
	wg.Wait()
}
