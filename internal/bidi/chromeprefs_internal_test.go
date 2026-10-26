package bidi

import "testing"

// A caller's preferences are kept, but none of them turns preloading back on.
func TestChromePrefsKeepPreloadingOff(t *testing.T) {
	got := chromePrefs(map[string]any{
		"net":    map[string]any{"network_prediction_options": 0, "other": true},
		"webkit": map[string]any{"webprefs": map[string]any{"default_font_size": 32}},
	})
	net, _ := got["net"].(map[string]any)
	if net["network_prediction_options"] != 2 || net["other"] != true {
		t.Errorf("net prefs = %v", net)
	}
	if w, _ := got["webkit"].(map[string]any); w["webprefs"] == nil {
		t.Errorf("the caller's other prefs were dropped: %v", got)
	}
	if n, _ := chromePrefs(nil)["net"].(map[string]any); n["network_prediction_options"] != 2 {
		t.Errorf("no caller prefs: %v", n)
	}
}
