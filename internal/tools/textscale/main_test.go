package main

import "testing"

func TestShotName(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:1313/":             "home-light.png",
		"http://127.0.0.1:1313/scan/":        "scan-light.png",
		"http://127.0.0.1:1313/for/qa-lead/": "for-qa-lead-light.png",
		"http://127.0.0.1:1313/404.html":     "404-light.png",
	} {
		if got := shotName(in, "light"); got != want {
			t.Errorf("shotName(%q) = %q, want %q", in, got, want)
		}
	}
}
