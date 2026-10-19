// Package watch polls a directory and hands over each new run record once it stops changing.
package watch

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Poller scans Dir every Interval for *.json files.
type Poller struct {
	Dir      string
	Interval time.Duration
	// IncludeExisting exports files that are already present at the first poll. Otherwise they
	// are treated as already exported.
	IncludeExisting bool
}

type state struct {
	size    int64
	modTime time.Time
	done    bool
}

// Run polls until ctx is cancelled. A file is handed to handle once it has kept the same size and
// modification time across two consecutive polls, so a record still being written is not read
// half way. Each file is handled once, whatever handle returns. Run returns ctx.Err().
func (p *Poller) Run(ctx context.Context, handle func(path string)) error {
	interval := p.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	seen := map[string]*state{}
	first := true
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		p.scan(seen, first, handle)
		first = false
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Poller) scan(seen map[string]*state, first bool, handle func(path string)) {
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(p.Dir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		st, ok := seen[path]
		if !ok {
			seen[path] = &state{size: info.Size(), modTime: info.ModTime(), done: first && !p.IncludeExisting}
			continue
		}
		if st.done {
			continue
		}
		if st.size != info.Size() || !st.modTime.Equal(info.ModTime()) {
			st.size, st.modTime = info.Size(), info.ModTime()
			continue
		}
		st.done = true
		handle(path)
	}
}
