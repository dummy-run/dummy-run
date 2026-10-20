package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseTextScales(t *testing.T) {
	for in, want := range map[string][]int{"": nil, "200": {200}, "100": {100}, "100,200": {100, 200}, "150% ,300": {150, 300}} {
		got, err := parseTextScales(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, %v want %v", in, got, err, want)
		}
	}
	for in, want := range map[string]string{
		"200,200": "given twice", "100,100": "given twice", "200,": "empty", ",200": "empty", "big": "not a percentage",
		"99": "outside 100 to 300", "301": "outside 100 to 300", "200,50": "outside 100 to 300",
	} {
		if _, err := parseTextScales(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want one with %q", in, err, want)
		}
	}
}

// quiet runs a11yCmd with standard error silenced, for calls that must be refused before a browser starts.
func quiet(t *testing.T, args ...string) int {
	t.Helper()
	old := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devnull
	defer func() { os.Stderr = old; _ = devnull.Close() }()
	return a11yCmd(args)
}

func TestCheckScreenshotFlags(t *testing.T) {
	for name, c := range map[string]struct {
		dir    string
		layout bool
		scales []int
		ok     bool
	}{
		"no screenshots":                {"", false, nil, true},
		"with layout":                   {"shots", true, nil, true},
		"with a text scale of 200":      {"shots", false, []int{200}, true},
		"with 100 and 200":              {"shots", false, []int{100, 200}, true},
		"with no layout checks":         {"shots", false, nil, false},
		"with a text scale of only 100": {"shots", false, []int{100}, false},
	} {
		if err := checkScreenshotFlags(c.dir, c.layout, c.scales); (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok %v", name, err, c.ok)
		}
	}
}

func TestA11yRefusesBadTextScaleAndScreenshotFlags(t *testing.T) {
	for name, args := range map[string][]string{
		"duplicate scales":                    {"--text-scale", "200,200", "http://127.0.0.1:1/"},
		"scale out of range":                  {"--text-scale", "400", "http://127.0.0.1:1/"},
		"screenshots with no layout checks":   {"--screenshots", "shots", "http://127.0.0.1:1/"},
		"screenshots at scale 100 only":       {"--screenshots", "shots", "--text-scale", "100", "http://127.0.0.1:1/"},
		"screenshots after the URL, no scale": {"http://127.0.0.1:1/", "--screenshots", "shots"},
	} {
		if code := quiet(t, args...); code != 2 {
			t.Errorf("%s: exit %d, want 2 before any browser starts", name, code)
		}
	}
}
