package page

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed socketblock.js
var socketBlockJS string

// blockSocketEvents installs the socket.io filter in every document the browser loads from now on. Each event it
// drops is reported on a channel and recorded like a blocked request.
func (p *Page) blockSocketEvents(ctx context.Context, patterns []string) error {
	mark, err := lookMark()
	if err != nil {
		return err
	}
	channel := "dr-socket-" + mark
	list, err := json.Marshal(patterns)
	if err != nil {
		return err
	}
	p.conn.OnScriptMessage(func(m bidi.ScriptMessage) {
		if m.Channel != channel {
			return
		}
		name, err := m.Data.String()
		if err != nil {
			return
		}
		p.mu.Lock()
		p.blocked = append(p.blocked, fmt.Sprintf("socket.io event %q (the target's block list)", clip(name, 80)))
		p.mu.Unlock()
	})
	if err := p.conn.Subscribe(ctx, []string{"script.message"}); err != nil {
		return err
	}
	decl := strings.Replace(socketBlockJS, "__DR_SOCKET_PATTERNS__", string(list), 1)
	_, err = p.conn.AddPreloadScriptWithChannel(ctx, decl, channel)
	return err
}

// clip shortens s to at most n runes.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
