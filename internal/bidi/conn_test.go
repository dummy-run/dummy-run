package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
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

// A chord whose input.performActions call fails, or runs out of time, still has its held keys released, and the
// caller gets the call's own error, with the release's when that fails too. A key with no modifier held needs no release.
func TestFailedChordReleasesHeldKeys(t *testing.T) {
	type answer struct{ fail, silent, failRelease bool }
	var mu sync.Mutex
	var methods []string
	var perform answer
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
				ID     int64  `json:"id"`
				Method string `json:"method"`
			}
			_ = json.Unmarshal(data, &cmd)
			mu.Lock()
			methods = append(methods, cmd.Method)
			a := perform
			mu.Unlock()
			reply := map[string]any{"type": "success", "id": cmd.ID, "result": map[string]any{}}
			if cmd.Method == "input.releaseActions" && a.failRelease {
				reply = map[string]any{"type": "error", "id": cmd.ID, "error": "unknown error", "message": "the release failed"}
			}
			if cmd.Method == "input.performActions" {
				if a.silent {
					continue
				}
				if a.fail {
					reply = map[string]any{"type": "error", "id": cmd.ID, "error": "unknown error", "message": "the keys stuck"}
				}
			}
			b, _ := json.Marshal(reply)
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
	try := func(a answer, callCtx context.Context, mods []string) ([]string, error) {
		mu.Lock()
		methods, perform = nil, a
		mu.Unlock()
		err := c.PressChord(callCtx, "cx", mods, KeyTab)
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(methods), err
	}
	got, err := try(answer{fail: true}, ctx, []string{KeyShift})
	if err == nil || !strings.Contains(err.Error(), "the keys stuck") {
		t.Errorf("error %v, want the call's own", err)
	}
	if want := []string{"input.performActions", "input.releaseActions"}; !slices.Equal(got, want) {
		t.Errorf("a failed chord sent %q, want %q", got, want)
	}
	short, stop := context.WithTimeout(ctx, 200*time.Millisecond)
	defer stop()
	got, err = try(answer{silent: true}, short, []string{KeyShift})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %v, want the call's deadline", err)
	}
	if want := []string{"input.performActions", "input.releaseActions"}; !slices.Equal(got, want) {
		t.Errorf("a chord that ran out of time sent %q, want %q", got, want)
	}
	_, err = try(answer{fail: true, failRelease: true}, ctx, []string{KeyShift})
	if err == nil || !strings.Contains(err.Error(), "the keys stuck") || !strings.Contains(err.Error(), "the release failed") {
		t.Errorf("error %v, want both the call's and the release's", err)
	}
	if err != nil && strings.Contains(err.Error(), "\n") {
		t.Errorf("error %q is on more than one line, and a page action keeps only the first", err)
	}
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Message != "the keys stuck" {
		t.Errorf("the call's own error isn't first to errors.As: %v", err)
	}
	got, err = try(answer{fail: true}, ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "the keys stuck") {
		t.Errorf("error %v, want the call's own", err)
	}
	if want := []string{"input.performActions"}; !slices.Equal(got, want) {
		t.Errorf("a failed key with no modifier sent %q, want %q", got, want)
	}
}
