package policy

import (
	"encoding/json"
	"testing"
)

func blockTarget() Target {
	t := target(false)
	t.Block = &Block{
		Requests: []RequestRule{
			{Method: "POST", Path: "/api/support/tickets"},
			{Method: "POST", Path: "/api/support/tickets/*/messages"},
			{Path: "/api/admin/**"},
			{Method: "DELETE", Path: "/api/reminders/*"},
		},
		SocketEvents: []string{"message:feedback", "feedback:*"},
	}
	return t
}

func TestDecideBlockRules(t *testing.T) {
	bt := blockTarget()
	cases := []struct {
		name, method, url string
		want              Verdict
	}{
		{"exact path and method", "POST", "https://www.example.test/api/support/tickets", BlockRule},
		{"trailing slash", "POST", "https://www.example.test/api/support/tickets/", BlockRule},
		{"query ignored", "POST", "https://www.example.test/api/support/tickets?x=1", BlockRule},
		{"path case ignored", "POST", "https://www.example.test/API/Support/Tickets", BlockRule},
		{"method case ignored", "post", "https://www.example.test/api/support/tickets", BlockRule},
		{"other method passes", "GET", "https://www.example.test/api/support/tickets", Allow},
		{"star matches one segment", "POST", "https://www.example.test/api/support/tickets/abc-123/messages", BlockRule},
		{"star does not cross a slash", "POST", "https://www.example.test/api/support/tickets/a/b/messages", Allow},
		{"star needs a segment", "POST", "https://www.example.test/api/support/tickets//messages", Allow},
		{"double star matches the rest", "GET", "https://www.example.test/api/admin/cases/7/notes", BlockRule},
		{"double star matches one segment", "PUT", "https://example.test/api/admin/x", BlockRule},
		{"double star needs something after it", "GET", "https://example.test/api/admin", Allow},
		{"no method means any method", "GET", "https://example.test/api/admin/x", BlockRule},
		{"delete reminder", "DELETE", "https://example.test/api/reminders/42", BlockRule},
		{"other path passes", "POST", "https://example.test/api/chat", Allow},
		{"prefix is not a match", "POST", "https://example.test/api/support/ticketsx", Allow},
		{"encoded slash does not split a segment", "POST", "https://example.test/api/support/tickets/a%2Fb/messages", BlockRule},
		{"percent-encoded letter still matches", "POST", "https://example.test/api/support/%74ickets", BlockRule},
		{"percent-encoded upper case still matches", "POST", "https://example.test/api/%53upport/tickets", BlockRule},
		{"doubled leading slash still matches", "POST", "https://example.test//api/support/tickets", BlockRule},
		{"doubled inner slash still matches", "POST", "https://example.test/api//support/tickets", BlockRule},
	}
	for _, c := range cases {
		if got := bt.Decide(c.method, c.url); got != c.want {
			t.Errorf("%s: Decide(%s %s) = %v, want %v", c.name, c.method, c.url, got, c.want)
		}
	}
}

// A rule never opens anything: a host outside the list stays blocked, and a read-only target still blocks writes
// the rules do not name.
func TestBlockRulesOnlyAddBlocks(t *testing.T) {
	bt := blockTarget()
	if got := bt.Decide("POST", "https://other.test/api/support/tickets"); got != BlockHost {
		t.Errorf("other host: got %v, want %v", got, BlockHost)
	}
	bt.ReadOnly = true
	if got := bt.Decide("POST", "https://example.test/api/chat"); got != BlockWrite {
		t.Errorf("read-only write: got %v, want %v", got, BlockWrite)
	}
}

func TestValidateBlock(t *testing.T) {
	if err := blockTarget().Validate(); err != nil {
		t.Fatalf("valid block rejected: %v", err)
	}
	bad := []*Block{
		{Requests: []RequestRule{{Path: "api/x"}}},
		{Requests: []RequestRule{{Path: ""}}},
		{Requests: []RequestRule{{Method: "PO ST", Path: "/x"}}},
		{Requests: []RequestRule{{Path: "/a/**/b"}}},
		{Requests: []RequestRule{{Path: "/a/[x"}}},
		{Requests: []RequestRule{{Path: "/a/x?y=1"}}},
		{SocketEvents: []string{""}},
		{SocketEvents: []string{"feedback:[x"}},
	}
	for i, b := range bad {
		tt := target(false)
		tt.Block = b
		if err := tt.Validate(); err == nil {
			t.Errorf("case %d accepted: %+v", i, b)
		}
	}
}

func TestSocketEventBlocked(t *testing.T) {
	bt := blockTarget()
	for name, want := range map[string]bool{
		"message:feedback": true,
		"feedback:submit":  true,
		"feedback:pilot":   true,
		"Feedback:Submit":  false,
		"message:send":     false,
		"feedback":         false,
	} {
		if got := bt.Block.SocketEventBlocked(name); got != want {
			t.Errorf("SocketEventBlocked(%q) = %v, want %v", name, got, want)
		}
	}
	var none *Block
	if none.SocketEventBlocked("feedback:submit") {
		t.Error("a nil block blocked an event")
	}
}

func TestBlockFromJSON(t *testing.T) {
	var tt Target
	raw := `{"adapter":"url","environment":"production","base_url":"https://example.test/","allowed_hosts":["example.test"],
"read_only":false,"block":{"requests":[{"method":"POST","path":"/api/referrals"}],"socket_events":["feedback:*"]}}`
	if err := json.Unmarshal([]byte(raw), &tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := tt.Decide("POST", "https://example.test/api/referrals"); got != BlockRule {
		t.Errorf("got %v, want %v", got, BlockRule)
	}
	if !tt.Block.SocketEventBlocked("feedback:submit") {
		t.Error("socket event from JSON not blocked")
	}
}

func TestBlockRuleVerdictString(t *testing.T) {
	if got := BlockRule.String(); got != "the target's block list" {
		t.Errorf("got %q", got)
	}
}
