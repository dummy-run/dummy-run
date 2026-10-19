package bidi

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
		typ  string
	}{
		{"success", `{"id":1,"type":"success","result":{"a":1}}`, true, "success"},
		{"error", `{"id":2,"type":"error","error":"no such element","message":"gone"}`, true, "error"},
		{"error without id", `{"type":"error","error":"invalid argument","message":"bad"}`, true, "error"},
		{"event", `{"type":"event","method":"log.entryAdded","params":{"level":"error"}}`, true, "event"},
		{"success without id", `{"type":"success","result":{}}`, false, ""},
		{"event without method", `{"type":"event","params":{}}`, false, ""},
		{"unknown type", `{"id":1,"type":"something"}`, false, ""},
		{"not json", `{`, false, ""},
		{"not an object", `[1,2]`, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Decode([]byte(tc.in))
			if tc.ok != (err == nil) {
				t.Fatalf("ok=%v err=%v", tc.ok, err)
			}
			if err != nil && !errors.Is(err, ErrDecode) {
				t.Errorf("error does not wrap ErrDecode: %v", err)
			}
			if m.Type != tc.typ {
				t.Errorf("type %q, want %q", m.Type, tc.typ)
			}
		})
	}
}

// FuzzDecode checks that no frame makes the decoder panic, and that a frame it
// accepts round-trips through the Message type.
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		`{"id":1,"type":"success","result":{"data":"iVBOR"}}`,
		`{"id":7,"type":"error","error":"unknown command","message":"x"}`,
		`{"type":"event","method":"network.beforeRequestSent","params":{"isBlocked":true,"request":{"request":"r1","headers":[{"name":"Host","value":{"type":"string","value":"a"}}]}}}`,
		`{"type":"event","method":"log.entryAdded","params":{"level":"error","text":"boom"}}`,
		`{}`, `[]`, `null`, `"x"`, `{"id":"1","type":"success"}`, `{"id":1e400,"type":"success"}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Decode(data)
		if err != nil {
			if !errors.Is(err, ErrDecode) {
				t.Fatalf("error does not wrap ErrDecode: %v", err)
			}
			return
		}
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		again, err := Decode(out)
		if err != nil {
			t.Fatalf("re-decode: %v", err)
		}
		if again.Type != m.Type || again.Method != m.Method || again.Error != m.Error {
			t.Fatalf("round trip changed the message: %+v -> %+v", m, again)
		}
	})
}

func TestCommandError(t *testing.T) {
	err := error(&CommandError{Method: "script.evaluate", Code: "no such frame", Message: "gone"})
	if !IsCommandError(err, "no such frame") || IsCommandError(err, "other") {
		t.Error("IsCommandError does not match on the code")
	}
	if err.Error() != "bidi: script.evaluate failed: no such frame: gone" {
		t.Errorf("message %q", err.Error())
	}
}

// An error frame without an id, which the remote end sends when it cannot parse
// a command, must be dropped rather than dereferenced.
func TestDispatchDropsErrorWithoutID(t *testing.T) {
	c := &Conn{pending: map[int64]chan Message{}, handlers: map[string][]EventHandler{}}
	m, err := Decode([]byte(`{"type":"error","error":"invalid argument","message":"unparsable"}`))
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(m) // must not panic
	id := int64(4)
	ch := make(chan Message, 1)
	c.pending[id] = ch
	c.dispatch(Message{ID: &id, Type: "success"})
	if len(ch) != 1 || len(c.pending) != 0 {
		t.Error("a matched result was not delivered")
	}
}
