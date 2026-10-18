package bidi

import "encoding/json"

// LogEntry is a log.entryAdded event: a console call or a script error.
type LogEntry struct {
	Type      string `json:"type"`
	Level     string `json:"level"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
	Source    struct {
		Context string `json:"context"`
	} `json:"source"`
	Method string `json:"method,omitempty"`
}

// OnLogEntry calls f for every log.entryAdded event. Call Subscribe with
// "log.entryAdded" first.
func (c *Conn) OnLogEntry(f func(LogEntry)) {
	c.OnEvent("log.entryAdded", func(_ string, params json.RawMessage) {
		var e LogEntry
		if err := json.Unmarshal(params, &e); err != nil {
			return
		}
		f(e)
	})
}
