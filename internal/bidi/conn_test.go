package bidi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// A call whose context has already ended fails on its own and leaves the connection open for the next call. The
// websocket closes the whole connection when a write's context ends, so the write mustn't use the call's.
func TestEndedCallLeavesTheConnectionOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		for {
			_, data, err := ws.Read(r.Context())
			if err != nil {
				return
			}
			var cmd struct {
				ID int64 `json:"id"`
			}
			_ = json.Unmarshal(data, &cmd)
			b, _ := json.Marshal(map[string]any{"type": "success", "id": cmd.ID, "result": map[string]any{}})
			if err := ws.Write(r.Context(), websocket.MessageText, b); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.ws.CloseNow() }()
	ended, stop := context.WithCancel(ctx)
	stop()
	if err := c.Call(ended, "session.status", nil, nil); err == nil {
		t.Fatal("a call with an ended context succeeded")
	}
	if err := c.Call(ctx, "session.status", nil, nil); err != nil {
		t.Fatalf("the next call failed: %v", err)
	}
}
