package lighthouse

import "testing"

const report = `{"lighthouseVersion":"13.5.0",
 "categories":{
  "accessibility":{"score":0.82,"auditRefs":[{"id":"image-alt"},{"id":"label"},{"id":"accesskeys"}]},
  "best-practices":{"score":0.85,"auditRefs":[{"id":"paste-preventing-inputs"},{"id":"charset"},{"id":"errors-in-console"}]}},
 "audits":{
  "image-alt":{"title":"Image elements do not have [alt] attributes","score":0,"scoreDisplayMode":"binary",
   "details":{"items":[{"node":{"selector":"body > main > img","nodeLabel":"img","explanation":"Fix any of the following:\n  Element does not have an alt attribute"}}]}},
  "label":{"title":"Form elements have associated labels","score":1,"scoreDisplayMode":"binary"},
  "accesskeys":{"title":"accesskeys","score":null,"scoreDisplayMode":"notApplicable"},
  "paste-preventing-inputs":{"title":"Prevents users from pasting into input fields","score":0,"scoreDisplayMode":"binary",
   "details":{"items":[{"node":{"selector":"body > main > input","nodeLabel":"Password"}}]}},
  "charset":{"title":"Charset declaration is missing or occurs too late in the HTML","score":0,"scoreDisplayMode":"binary","details":{"items":[]}},
  "errors-in-console":{"title":"Browser errors were logged to the console","score":0,"scoreDisplayMode":"informative"}}}`

func TestParseKeepsFailedScoredAudits(t *testing.T) {
	r, err := Parse([]byte(report))
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "13.5.0" || r.Scores["accessibility"] != 0.82 || r.Scores["best-practices"] != 0.85 {
		t.Errorf("version and scores: %+v", r)
	}
	var ids []string
	for _, f := range r.Failures {
		ids = append(ids, f.Category+"/"+f.ID)
	}
	want := []string{"accessibility/image-alt", "best-practices/paste-preventing-inputs", "best-practices/charset"}
	if len(ids) != len(want) {
		t.Fatalf("failures %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("failure %d is %s, want %s", i, ids[i], want[i])
		}
	}
	if it := r.Failures[0].Items[0]; it.Selector != "body > main > img" || it.Detail != "Fix any of the following:" {
		t.Errorf("item %+v", it)
	}
}

func TestParseReportsARuntimeError(t *testing.T) {
	if _, err := Parse([]byte(`{"runtimeError":{"code":"ERRORED_DOCUMENT_REQUEST","message":"Lighthouse was unable to load the page"}}`)); err == nil {
		t.Error("a runtime error was not reported")
	}
}
