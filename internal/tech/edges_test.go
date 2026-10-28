package tech

import (
	"encoding/json"
	"testing"
)

// TestFingerprintKeepsEachKindOfSignal checks a technology with any two neighbouring kinds of signal is kept, so no
// count of one kind can cancel out another's.
func TestFingerprintKeepsEachKindOfSignal(t *testing.T) {
	signals := []string{
		`"js": {"Shop": ""}`,
		`"scriptSrc": "shop\\.js"`,
		`"html": "<shop"`,
		`"meta": {"generator": "Shop"}`,
		`"cookies": {"shop_session": ""}`,
		`"dom": "#shop"`,
	}
	for i := 0; i+1 < len(signals); i++ {
		raw := "{" + signals[i] + ", " + signals[i+1] + "}"
		var w waTech
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if _, ok := w.fingerprint("Shop"); !ok {
			t.Errorf("%s was left out", raw)
		}
	}
	// The plain and checked dom forms come from one field, so they share an entry here.
	var w waTech
	if err := json.Unmarshal([]byte(`{"dom": {"#shop": {"exists": ""}, "#cart": {"text": "Cart"}}}`), &w); err != nil {
		t.Fatal(err)
	}
	if fp, ok := w.fingerprint("Shop"); !ok || len(fp.Selectors) != 1 || len(fp.DOM) != 1 {
		t.Errorf("selectors %v, checks %v, kept %v", fp.Selectors, fp.DOM, ok)
	}
	// A text check with no pattern asks for any text at all.
	var empty waTech
	if err := json.Unmarshal([]byte(`{"dom": {"#cart": {"text": ""}}}`), &empty); err != nil {
		t.Fatal(err)
	}
	if fp, _ := empty.fingerprint("Shop"); len(fp.DOM) != 1 || fp.DOM[0].Text != `\S` {
		t.Errorf("an empty text check became %+v", fp.DOM)
	}
	var none waTech
	if _, ok := none.fingerprint("Nothing"); ok {
		t.Error("a technology with no signals was kept")
	}
}

// TestPatternTags checks webappanalyzer's tags come off wherever they start, the start of the pattern included.
func TestPatternTags(t *testing.T) {
	for in, want := range map[string]string{
		`shop\;version:\1`: "shop",
		`\;confidence:50`:  "",
		`shop`:             "shop",
	} {
		if got := pattern(in); got != want {
			t.Errorf("pattern(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMetaLists checks a meta value given as a list uses its first entry, and an empty list adds nothing.
func TestMetaLists(t *testing.T) {
	var w waTech
	if err := json.Unmarshal([]byte(`{"meta": {"generator": ["Shop\\;version:\\1", "Other"], "empty": []}, "html": "<shop"}`), &w); err != nil {
		t.Fatal(err)
	}
	fp, ok := w.fingerprint("Shop")
	if !ok || fp.Meta["generator"] != "Shop" || len(fp.Meta) != 1 {
		t.Errorf("meta %v, kept %v", fp.Meta, ok)
	}
}
