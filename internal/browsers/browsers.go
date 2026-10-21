// Package browsers installs and finds the browsers Dummy Run drives: a pinned Chrome for Testing with its
// matching chromedriver, and a pinned Firefox on Linux. They live in a per-user cache, so a first run needs
// no hand-matched chromedriver.
//
// versions.json is the single source of truth for the pins and their SHA-256 hashes. A test checks it against
// .github/workflows/engine.yml and docker/Dockerfile. Chrome for Testing publishes no hashes, so the file
// holds the ones taken from the files at the pinned version, and every download is checked against them.
package browsers

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

//go:embed versions.json
var versionsJSON []byte

// Versions is the content of versions.json.
type Versions struct {
	ChromeForTesting string            `json:"chrome_for_testing"`
	Firefox          string            `json:"firefox"`
	Chrome           map[string]string `json:"chrome"`
	Chromedriver     map[string]string `json:"chromedriver"`
	FirefoxLinux     map[string]string `json:"firefox_linux"`
}

// Pins returns the pinned versions and hashes.
func Pins() Versions {
	var v Versions
	if err := json.Unmarshal(versionsJSON, &v); err != nil {
		panic("browsers: versions.json: " + err.Error()) // a build with a broken file is a bug, caught by the tests
	}
	return v
}

// CacheEnv names the variable that moves the cache, for tests and for machines with a small home directory.
const CacheEnv = "DUMMYRUN_BROWSER_CACHE"

// CacheRoot is the directory that holds the installed browsers: DUMMYRUN_BROWSER_CACHE, or
// os.UserCacheDir()/dummyrun/browsers.
func CacheRoot() (string, error) {
	if d := os.Getenv(CacheEnv); d != "" {
		return d, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("browsers: no cache directory (%w); set %s", err, CacheEnv)
	}
	return filepath.Join(base, "dummyrun", "browsers"), nil
}

// ChromePlatform is the Chrome for Testing name for this machine, or an error where there is no build.
func ChromePlatform() (string, error) { return chromePlatform(runtime.GOOS, runtime.GOARCH) }

func chromePlatform(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "linux64", nil
	case "linux/arm64":
		return "linux-arm64", nil
	case "darwin/arm64":
		return "mac-arm64", nil
	case "darwin/amd64":
		return "mac-x64", nil
	case "windows/amd64":
		return "win64", nil
	}
	return "", fmt.Errorf("browsers: Chrome for Testing has no build for %s/%s; install Chrome and chromedriver yourself and set DUMMYRUN_CHROME and DUMMYRUN_CHROMEDRIVER", goos, goarch)
}

func firefoxPlatform(goos, goarch string) (string, bool) {
	if goos != "linux" {
		return "", false
	}
	switch goarch {
	case "amd64":
		return "linux-x86_64", true
	case "arm64":
		return "linux-aarch64", true
	}
	return "", false
}

// exe adds .exe on Windows.
func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// chromeBinary is the path of the Chrome binary inside an extracted chrome-<platform> folder.
func chromeBinary(dir, platform string) string {
	switch {
	case strings.HasPrefix(platform, "mac"):
		return filepath.Join(dir, "chrome-"+platform, "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
	case platform == "win64":
		return filepath.Join(dir, "chrome-"+platform, "chrome.exe")
	}
	return filepath.Join(dir, "chrome-"+platform, "chrome")
}

func driverBinary(dir, platform string) string {
	return filepath.Join(dir, "chromedriver-"+platform, exe("chromedriver"))
}

func firefoxBinary(dir string) string { return filepath.Join(dir, "firefox", "firefox") }

// marker is written after a download is verified and unpacked. A folder without it is a failed install.
const marker = ".complete"

func complete(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, marker))
	return err == nil
}

// markerError returns an error when the completion marker cannot be read for a reason other than its absence,
// so a permission problem is reported instead of being taken for a missing install and wiped.
func markerError(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, marker)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("browsers: cannot read %s: %w", dir, err)
	}
	return nil
}

func isFile(p string) bool {
	fi, err := os.Stat(p) //nolint:gosec // paths come from the cache folder or the caller's own configuration
	return err == nil && !fi.IsDir()
}

// chromeDirs lists the installed Chrome folders in the cache, the pinned version first and then the others,
// newest name first.
func chromeDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	pinned := Pins().ChromeForTesting
	var others []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() && n != pinned && !strings.HasPrefix(n, "firefox-") && !strings.HasPrefix(n, ".") && !strings.HasSuffix(n, ".partial") {
			others = append(others, n)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(others)))
	return append([]string{pinned}, others...)
}

// CachedChrome returns the Chrome and chromedriver of the cached pair for this machine, or ok false. A pair
// installed at another version is used when the pinned one is missing, since the two always match.
func CachedChrome() (chrome, driver string, ok bool) {
	root, err := CacheRoot()
	if err != nil {
		return "", "", false
	}
	platform, err := ChromePlatform()
	if err != nil {
		return "", "", false
	}
	for _, v := range chromeDirs(root) {
		dir := filepath.Join(root, v)
		if complete(dir) && isFile(chromeBinary(dir, platform)) && isFile(driverBinary(dir, platform)) {
			return chromeBinary(dir, platform), driverBinary(dir, platform), true
		}
	}
	return "", "", false
}

// CachedFirefox returns the Firefox binary this tool installed, or ok false.
func CachedFirefox() (string, bool) {
	root, err := CacheRoot()
	if err != nil {
		return "", false
	}
	dir := filepath.Join(root, "firefox-"+Pins().Firefox)
	if complete(dir) && isFile(firefoxBinary(dir)) {
		return firefoxBinary(dir), true
	}
	return "", false
}
