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

func TestWantRoot(t *testing.T) {
	if got := wantRoot(32, 0); got != 32 {
		t.Errorf("wantRoot(32, 0) = %v", got)
	}
	if got := wantRoot(16, 200); got != 32 {
		t.Errorf("wantRoot(16, 200) = %v", got)
	}
	if got := wantRoot(24, 150); got != 36 {
		t.Errorf("wantRoot(24, 150) = %v", got)
	}
}

func TestMergeOpen(t *testing.T) {
	res := result{ViewportWidth: 320, PageScrollWidth: 320}
	mergeOpen(&res, result{Overlaps: []string{"a overlaps b"}, ViewportWidth: 320, PageScrollWidth: 400})
	if len(res.Overlaps) != 1 || res.Overlaps[0] != "menu open: a overlaps b" {
		t.Errorf("overlaps = %v", res.Overlaps)
	}
	if len(res.Overflow) != 1 {
		t.Errorf("a page that only scrolls with the menu open should be reported once: %v", res.Overflow)
	}
	res = result{ViewportWidth: 320, PageScrollWidth: 400}
	mergeOpen(&res, result{ViewportWidth: 320, PageScrollWidth: 400})
	if len(res.Overflow) != 0 {
		t.Errorf("a page that scrolls anyway is reported by the first check: %v", res.Overflow)
	}
}
