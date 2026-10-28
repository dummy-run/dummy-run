package a11y

import (
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/plain"
)

func TestPlainFindingsGroupAndAllow(t *testing.T) {
	allow, err := ParseAllowList(strings.NewReader("dummyrun-plain-latin | /help | main | our legal text uses etc.\n"))
	if err != nil {
		t.Fatal(err)
	}
	alerts := []plain.Alert{
		{Check: "Readability.FleschReadingEase", Message: "Try to keep the Flesch reading ease score (52.1) above 70.", Line: 1, Link: "https://example.org/fre"},
		{Check: "DummyRun.WordsToAvoid", Message: "Try 'use' instead of 'leverage'.", Match: "leverage", Line: 2},
		{Check: "DummyRun.WordsToAvoid", Message: "Try 'start' instead of 'initiate'.", Match: "initiate", Line: 4},
		{Check: "DummyRun.Latin", Message: "Try 'and so on' instead of 'etc.'.", Match: "etc.", Line: 5},
		{Check: "Vale.Spelling", Message: "ignored", Line: 6},
		{Check: "DummyRun.LongSentence", Message: "This sentence has more than 25 words.", Match: "We", Line: 7},
	}
	got := plainFindings(alerts, "main", "/help", allow)
	var ids []string
	for _, f := range got {
		ids = append(ids, f.Rule)
	}
	if strings.Join(ids, ",") != "dummyrun-plain-reading-ease,dummyrun-plain-long-sentence,dummyrun-plain-words-to-avoid,dummyrun-plain-latin" {
		t.Fatalf("rules: %v", ids)
	}
	if got[0].HelpURL != "https://example.org/fre" || got[0].Nodes[0].Detail != "Try to keep the Flesch reading ease score (52.1) above 70." {
		t.Errorf("reading ease: %+v", got[0])
	}
	if got[1].Nodes[0].Detail != `a sentence that starts with "We" has more than 25 words` {
		t.Errorf("long sentence: %+v", got[1])
	}
	if len(got[2].Nodes) != 2 || got[2].Nodes[1].Target != "main" {
		t.Errorf("words to avoid: %+v", got[2])
	}
	if !got[3].Nodes[0].Allowed {
		t.Errorf("the allow-list did not accept the Latin finding: %+v", got[3])
	}
}
