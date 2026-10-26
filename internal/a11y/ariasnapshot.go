package a11y

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/page"
)

// maxChangeLines is the most added and removed lines a result lists. The counts cover them all.
const maxChangeLines = 50

// ARIAChanges are the lines of a page's ARIA snapshot that differ from the snapshot left by an earlier sweep.
type ARIAChanges struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	// AddedCount and RemovedCount count every changed line, including the ones past the first 50.
	AddedCount   int `json:"added_count"`
	RemovedCount int `json:"removed_count"`
}

// snapshotName is the file a page's snapshot goes in: a readable part from the address and setting, and a short
// hash of both, so two addresses that read the same never share a file.
func snapshotName(raw, setting string) string {
	slug := raw
	if u, err := url.Parse(raw); err == nil {
		slug = u.Host + u.Path
		if u.RawQuery != "" {
			slug += "_" + u.RawQuery
		}
	}
	clean := regexp.MustCompile(`[^A-Za-z0-9.-]+`).ReplaceAllString(slug+"--"+setting, "_")
	if len(clean) > 100 {
		clean = clean[:100]
	}
	sum := sha256.Sum256([]byte(raw + "\n" + setting))
	return strings.Trim(clean, "_") + "-" + hex.EncodeToString(sum[:4]) + ".yaml"
}

// snapshotCheck outlines the page, compares the outline with the one an earlier sweep left in dir, and writes the
// new one in its place. A page seen for the first time has a snapshot and no changes. A failure is recorded on the
// result and doesn't stop the page's other checks.
func snapshotCheck(ctx context.Context, conn *bidi.Conn, cx, dir string, res *PageResult) {
	snap, err := page.TakeARIASnapshot(ctx, conn, cx)
	if err != nil {
		res.ARIASnapshotError = firstLine(err.Error())
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		res.ARIASnapshotError = firstLine(err.Error())
		return
	}
	path := filepath.Join(dir, snapshotName(res.URL, res.Setting()))
	old, err := os.ReadFile(path) //nolint:gosec // a file this sweep names inside the directory you chose
	switch {
	case err == nil:
		res.ARIAChanges = diffLines(splitLines(string(old)), snap.Lines)
	case !errors.Is(err, fs.ErrNotExist):
		res.ARIASnapshotError = firstLine(err.Error())
		return
	}
	if err := replaceFile(dir, path, []byte(snap.Text())); err != nil {
		res.ARIASnapshotError = firstLine(err.Error())
		return
	}
	res.ARIASnapshot, res.ARIASnapshotOmitted = path, snap.Omitted
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diffLines lists the lines only in b as added and the lines only in a as removed, by a longest common
// subsequence, so a line that moved shows once on each side.
func diffLines(a, b []string) *ARIAChanges {
	n, m := len(a), len(b)
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	c := &ARIAChanges{Added: []string{}, Removed: []string{}}
	add := func(list *[]string, count *int, line string) {
		*count++
		if len(*list) < maxChangeLines {
			*list = append(*list, line)
		}
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			i, j = i+1, j+1
		case j < m && (i == n || lcs[i][j+1] >= lcs[i+1][j]):
			add(&c.Added, &c.AddedCount, b[j])
			j++
		default:
			add(&c.Removed, &c.RemovedCount, a[i])
			i++
		}
	}
	return c
}

// snapshotItem is a snapshot line without its indent and list marker, for the terminal, where - and + mark the change.
func snapshotItem(line string) string { return strings.TrimPrefix(strings.TrimSpace(line), "- ") }

// replaceFile writes data to a new file in dir and renames it over path, so a write that fails part way leaves the
// earlier snapshot whole for the next sweep to compare with.
func replaceFile(dir, path string, data []byte) error {
	f, err := os.CreateTemp(dir, ".snapshot-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
