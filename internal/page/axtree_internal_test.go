package page

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAXTableKeepsWhatNamesJSUses(t *testing.T) {
	var nodes []axNode
	if err := json.Unmarshal([]byte(`[
		{"role":{"value":"button"},"name":{"value":"Search"},"backendDOMNodeId":7,
		 "properties":[{"name":"expanded","value":{"value":false}},{"name":"disabled","value":{"value":true}},{"name":"invalid","value":{"value":"spelling"}}]},
		{"role":{"value":"image"},"name":{"value":"Logo"},"backendDOMNodeId":8,"ignored":true},
		{"role":{"value":"DisclosureTriangle"},"name":{"value":"More"},"backendDOMNodeId":9},
		{"role":{"value":"StaticText"},"name":{"value":"Search"},"backendDOMNodeId":10},
		{"role":{"value":"generic"},"name":{"value":""},"backendDOMNodeId":11},
		{"role":{"value":"link"},"name":{"value":"No node"}}
	]`), &nodes); err != nil {
		t.Fatal(err)
	}
	got := axTable(nodes)
	if len(got) != 3 {
		t.Fatalf("want three entries, got %+v", got)
	}
	if e := got[7]; *e.R != "button" || e.N != "Search" || !reflect.DeepEqual(e.S, []string{"disabled", "collapsed", "invalid"}) {
		t.Errorf("button: %+v", e)
	}
	if e := got[8]; *e.R != "img" || !e.I {
		t.Errorf("image: %+v", e)
	}
	if e := got[9]; e.R != nil || e.N != "More" {
		t.Errorf("Chrome's own role should be left to names.js: %+v", e)
	}
}
