package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// DefaultTimeout bounds one command when the caller's context has no deadline.
const DefaultTimeout = 30 * time.Second

// readLimit is the largest frame the client accepts. Screenshots of a tall
// page arrive as base64 in one frame.
const readLimit = 128 << 20

// EventHandler receives one event. Handlers run on the read loop, so they
// must return quickly and never call back into the connection synchronously.
type EventHandler func(method string, params json.RawMessage)

// Conn is one WebSocket connection to a BiDi remote end.
type Conn struct {
	ws      *websocket.Conn
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan Message
	// handlers maps an event method to its handlers. The empty key receives every event. nextHandler numbers them,
	// so one can be removed.
	handlers    map[string][]handler
	nextHandler uint64
	closed      chan struct{}
	readErr     error
	onceErr     sync.Once
}

// Dial opens a connection to url and starts reading frames.
func Dial(ctx context.Context, url string) (*Conn, error) {
	ws, _, err := websocket.Dial(ctx, url, nil) //nolint:bodyclose // the response body is closed by the library on a successful upgrade
	if err != nil {
		return nil, fmt.Errorf("bidi: dial %s: %w", url, err)
	}
	ws.SetReadLimit(readLimit)
	c := &Conn{
		ws:       ws,
		pending:  map[int64]chan Message{},
		handlers: map[string][]handler{},
		closed:   make(chan struct{}),
	}
	go c.readLoop() //nolint:gosec // the read loop outlives the dial context by design
	return c, nil
}

func (c *Conn) readLoop() {
	defer close(c.closed)
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.fail(fmt.Errorf("bidi: connection closed: %w", err))
			return
		}
		m, err := Decode(data)
		if err != nil {
			// A frame the client cannot read is dropped, not fatal: the remote end
			// may send extensions this client does not know.
			continue
		}
		c.dispatch(m)
	}
}

func (c *Conn) dispatch(m Message) {
	c.mu.Lock()
	if m.Type == "event" {
		hs := append([]handler{}, c.handlers[""]...)
		hs = append(hs, c.handlers[m.Method]...)
		c.mu.Unlock()
		for _, h := range hs {
			h.f(m.Method, m.Params)
		}
		return
	}
	if m.ID == nil {
		// An error the remote end could not tie to a command. Nothing waits for it.
		c.mu.Unlock()
		return
	}
	ch := c.pending[*m.ID]
	delete(c.pending, *m.ID)
	c.mu.Unlock()
	if ch != nil {
		ch <- m
	}
}

func (c *Conn) fail(err error) {
	c.onceErr.Do(func() {
		c.mu.Lock()
		c.readErr = err
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
	})
}

type handler struct {
	id uint64
	f  EventHandler
}

// OnEvent registers a handler for an event method. An empty method receives
// every event. The returned function removes the handler; calling it again does nothing.
func (c *Conn) OnEvent(method string, h EventHandler) (remove func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextHandler++
	id := c.nextHandler
	c.handlers[method] = append(c.handlers[method], handler{id: id, f: h})
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		hs := c.handlers[method]
		for i := range hs {
			if hs[i].id == id {
				c.handlers[method] = append(hs[:i:i], hs[i+1:]...)
				return
			}
		}
	}
}

// Call sends one command and decodes its result into out, which may be nil.
func (c *Conn) Call(ctx context.Context, method string, params any, out any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	if params == nil {
		params = map[string]any{}
	}
	c.mu.Lock()
	if c.readErr != nil {
		c.mu.Unlock()
		return c.readErr
	}
	c.nextID++
	id := c.nextID
	ch := make(chan Message, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	body, err := json.Marshal(struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{id, method, params})
	if err != nil {
		c.forget(id)
		return fmt.Errorf("bidi: encode %s: %w", method, err)
	}
	// The websocket closes the whole connection when a write's context ends, so a call whose own time ran out would
	// take the browser down for every other caller. The write gets a context of its own, and the call's context
	// only bounds the wait for the answer.
	if err := ctx.Err(); err != nil {
		c.forget(id)
		return fmt.Errorf("bidi: %s: %w", method, err)
	}
	wctx, wcancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultTimeout)
	err = c.ws.Write(wctx, websocket.MessageText, body)
	wcancel()
	if err != nil {
		c.forget(id)
		return fmt.Errorf("bidi: send %s: %w", method, err)
	}
	select {
	case m, ok := <-ch:
		if !ok {
			c.mu.Lock()
			err := c.readErr
			c.mu.Unlock()
			return err
		}
		if m.Type == "error" {
			return &CommandError{Method: method, Code: m.Error, Message: m.ErrorMessage}
		}
		if out == nil || len(m.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(m.Result, out); err != nil {
			return fmt.Errorf("bidi: decode %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.forget(id)
		return fmt.Errorf("bidi: %s: %w", method, ctx.Err())
	}
}

func (c *Conn) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Close ends the connection. Pending commands fail.
func (c *Conn) Close() error {
	err := c.ws.Close(websocket.StatusNormalClosure, "done")
	select {
	case <-c.closed:
	case <-time.After(2 * time.Second):
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return nil //nolint:nilerr // a close error after the peer has gone is not worth reporting
	}
	return nil
}
