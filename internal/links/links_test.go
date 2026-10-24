package links

import (
	"os"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		code    int
		timeout bool
		want    string
	}{{200, false, OK}, {301, false, OK}, {404, false, Broken}, {500, false, Broken}, {0, false, Broken},
		{403, false, Refused}, {429, false, Refused}, {999, false, Refused}, {0, true, TimedOut}} {
		if got := Classify(c.code, c.timeout); got != c.want {
			t.Errorf("Classify(%d, %v) = %s, want %s", c.code, c.timeout, got, c.want)
		}
	}
}

// A report in the shape lychee 0.24 writes.
const report = `{"total":5,"error_map":{"stdin":[
 {"url":"https://example.test/gone","status":{"text":"Rejected status code: 404 Not Found","code":404}},
 {"url":"https://example.test/wall","status":{"text":"Rejected status code: 403 Forbidden","code":403}},
 {"url":"https://nohost.test/","status":{"text":"Network error: dns error\nmore"}}]},
 "timeout_map":{"stdin":[{"url":"https://slow.test/","status":{"text":"Timeout"}}]},
 "excluded_map":{"stdin":[{"url":"https://x.invalid/","status":{"text":"Excluded"}}]}}`

func TestParse(t *testing.T) {
	urls := []string{"https://example.test/fine", "https://example.test/gone", "https://example.test/wall", "https://nohost.test", "https://slow.test/", "https://x.invalid/"}
	got, err := parse([]byte(report), urls)
	if err != nil {
		t.Fatal(err)
	}
	want := []Result{
		{URL: urls[0], Outcome: OK},
		{URL: urls[1], Outcome: Broken, Code: 404, Text: "Rejected status code: 404 Not Found"},
		{URL: urls[2], Outcome: Refused, Code: 403, Text: "Rejected status code: 403 Forbidden"},
		{URL: urls[3], Outcome: Broken, Text: "Network error: dns error"},
		{URL: urls[4], Outcome: TimedOut, Text: "Timeout"},
		{URL: urls[5], Outcome: Skipped},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestLycheeVersionMatchesDockerfile(t *testing.T) {
	b, err := os.ReadFile("../../docker/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\nARG LYCHEE_VERSION="+LycheeVersion+"\n") {
		t.Errorf("docker/Dockerfile does not pin lychee %s", LycheeVersion)
	}
}
