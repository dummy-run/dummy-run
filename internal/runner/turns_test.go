package runner

import (
	"testing"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/provider"
)

// When the model asks for two tools in one turn, both tool replies must come
// before the screenshot message, or the chat API rejects the conversation.
func TestTurnsKeepToolRepliesTogether(t *testing.T) {
	step := &Step{N: 2, PageChanged: true, Obs: page.Observation{Action: "click", URL: "https://t.test/", Screenshot: []byte("png"),
		Snapshot: &page.Snapshot{Title: "T"}}}
	r := &run{budget: 25, entries: []entry{
		{role: provider.RoleUser, text: "Begin.", step: &Step{N: 1, Obs: page.Observation{Action: "visit", Screenshot: []byte("png"), Snapshot: &page.Snapshot{}}}},
		{role: provider.RoleAssistant, calls: []provider.ToolCall{{ID: "c1", Name: "click"}, {ID: "c2", Name: "look"}}},
		{role: provider.RoleTool, callID: "c1", step: step, stepsLeft: 23},
		{role: provider.RoleTool, callID: "c2", text: "Not run: one action per turn."},
	}}
	turns := r.turns(provider.Capabilities{})
	roles := make([]string, 0, len(turns))
	for _, tn := range turns {
		roles = append(roles, string(tn.Role)+":"+tn.ToolCallID)
	}
	want := []string{"user:", "assistant:", "tool:c1", "tool:c2", "user:"}
	if len(roles) != len(want) {
		t.Fatalf("turns %v, want %v", roles, want)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("turns %v, want %v", roles, want)
		}
	}
	if len(turns[4].Images) != 1 {
		t.Error("the screenshot message lost its image")
	}
}

// A backend that takes images in tool results gets the screenshot inside the
// tool turn, with no separate screenshot message.
func TestTurnsPutScreenshotInToolResultWhenSupported(t *testing.T) {
	step := &Step{N: 2, PageChanged: true, Obs: page.Observation{Action: "click", URL: "https://t.test/", Screenshot: []byte("png"),
		Snapshot: &page.Snapshot{Title: "T"}}}
	r := &run{budget: 25, entries: []entry{
		{role: provider.RoleUser, text: "Begin.", step: &Step{N: 1, Obs: page.Observation{Action: "visit", Snapshot: &page.Snapshot{}}}},
		{role: provider.RoleAssistant, calls: []provider.ToolCall{{ID: "c1", Name: "click"}}},
		{role: provider.RoleTool, callID: "c1", step: step, stepsLeft: 23},
	}}
	turns := r.turns(provider.Capabilities{ImagesInToolResults: true})
	if len(turns) != 3 || turns[2].Role != provider.RoleTool || len(turns[2].Images) != 1 {
		t.Fatalf("turns %+v", turns)
	}
}
