package review

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// LogEntry is one line of review-log.jsonl: one decision about one problem, for the records in the
// directory of the log.
type LogEntry struct {
	At          time.Time `json:"at"`
	Reviewer    string    `json:"reviewer"`
	Fingerprint string    `json:"fingerprint"`
	Verdict     string    `json:"verdict"`
	Reason      string    `json:"reason,omitempty"`
	Severity    string    `json:"severity,omitempty"`
	Title       string    `json:"title,omitempty"`
	Applied     []Applied `json:"applied"`
}

// writeLog adds one line to the log in each directory that holds a changed record.
func writeLog(fingerprint string, d Decision, applied []Applied) error {
	byDir := map[string][]Applied{}
	var dirs []string
	for _, a := range applied {
		dir := filepath.Dir(a.File)
		if _, ok := byDir[dir]; !ok {
			dirs = append(dirs, dir)
		}
		a.File = filepath.Base(a.File) // the log sits next to the record, so the name is enough
		byDir[dir] = append(byDir[dir], a)
	}
	for _, dir := range dirs {
		line, err := json.Marshal(LogEntry{At: d.At, Reviewer: d.Reviewer, Fingerprint: fingerprint, Verdict: d.Verdict,
			Reason: d.Reason, Severity: d.Severity, Title: d.Title, Applied: byDir[dir]})
		if err != nil {
			return err
		}
		line = append(bytes.TrimSpace(line), '\n')
		f, err := os.OpenFile(filepath.Join(dir, LogName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640) //nolint:gosec // next to the records
		if err != nil {
			return err
		}
		// One write call, so two reviewers on one directory do not interleave a line.
		_, werr := f.Write(line)
		if serr := f.Sync(); werr == nil {
			werr = serr
		}
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
	}
	return nil
}
