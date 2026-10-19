package page

import (
	"reflect"
	"testing"
)

func TestRedact(t *testing.T) {
	in := "https://app.test/reset?token=abc123&page=2&api_key=zzz#frag"
	want := "https://app.test/reset?token=[hidden]&page=2&api_key=[hidden]#frag"
	if got := redact(in); got != want {
		t.Errorf("redact gave %q", got)
	}
	if got := redact("https://app.test/plain?q=1"); got != "https://app.test/plain?q=1" {
		t.Errorf("redact changed a harmless URL: %q", got)
	}
}

func TestUniqAndFirstLine(t *testing.T) {
	if got := uniq([]string{"a", "b", "a", "c", "b"}); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("uniq gave %v", got)
	}
	if got := firstLine("one\ntwo"); got != "one" {
		t.Errorf("firstLine gave %q", got)
	}
	long := make([]byte, 400)
	for i := range long {
		long[i] = 'x'
	}
	if got := firstLine(string(long)); len(got) != 300 {
		t.Errorf("firstLine kept %d bytes", len(got))
	}
}
