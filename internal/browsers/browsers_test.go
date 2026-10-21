package browsers

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Skipf("not running inside the repository: %v", err)
	}
	return string(data)
}

func match(t *testing.T, text, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no match for %q", pattern)
	}
	return m[1]
}

// TestPinsMatchWorkflowAndDockerfile keeps versions.json, the engine workflow and the Dockerfile on the same browsers.
func TestPinsMatchWorkflowAndDockerfile(t *testing.T) {
	p := Pins()
	wf := repoFile(t, ".github/workflows/engine.yml")
	df := repoFile(t, "docker/Dockerfile")
	if got := match(t, wf, `(?m)^  CHROME_FOR_TESTING: *(\S+)`); got != p.ChromeForTesting {
		t.Errorf("engine.yml pins Chrome %s, versions.json %s", got, p.ChromeForTesting)
	}
	if got := match(t, wf, `(?m)^  FIREFOX_VERSION: *"?([^"\s]+)"?`); got != p.Firefox {
		t.Errorf("engine.yml pins Firefox %s, versions.json %s", got, p.Firefox)
	}
	if got := match(t, df, `(?m)^ARG CHROME_FOR_TESTING=(\S+)`); got != p.ChromeForTesting {
		t.Errorf("Dockerfile pins Chrome %s, versions.json %s", got, p.ChromeForTesting)
	}
	if got := match(t, df, `(?m)^ARG FIREFOX_VERSION=(\S+)`); got != p.Firefox {
		t.Errorf("Dockerfile pins Firefox %s, versions.json %s", got, p.Firefox)
	}
	for arg, want := range map[string]string{
		"CHROME_SHA256_AMD64":       p.Chrome["linux64"],
		"CHROME_SHA256_ARM64":       p.Chrome["linux-arm64"],
		"CHROMEDRIVER_SHA256_AMD64": p.Chromedriver["linux64"],
		"CHROMEDRIVER_SHA256_ARM64": p.Chromedriver["linux-arm64"],
		"FIREFOX_SHA256_AMD64":      p.FirefoxLinux["linux-x86_64"],
		"FIREFOX_SHA256_ARM64":      p.FirefoxLinux["linux-aarch64"],
	} {
		if got := match(t, df, `(?m)^ARG `+arg+`=(\S+)`); got != want {
			t.Errorf("Dockerfile %s is %s, versions.json has %s", arg, got, want)
		}
	}
}

func TestEveryPlatformHasHashes(t *testing.T) {
	p := Pins()
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, plat := range []string{"linux64", "linux-arm64", "mac-arm64", "mac-x64", "win64"} {
		for kind, m := range map[string]map[string]string{"chrome": p.Chrome, "chromedriver": p.Chromedriver} {
			if !hex64.MatchString(m[plat]) {
				t.Errorf("%s hash for %s is %q, want 64 hex digits", kind, plat, m[plat])
			}
		}
	}
	for plat, h := range p.FirefoxLinux {
		if !hex64.MatchString(h) {
			t.Errorf("firefox hash for %s is %q", plat, h)
		}
	}
}

func TestChromePlatform(t *testing.T) {
	for pair, want := range map[string]string{"linux/amd64": "linux64", "linux/arm64": "linux-arm64", "darwin/arm64": "mac-arm64",
		"darwin/amd64": "mac-x64", "windows/amd64": "win64"} {
		goos, goarch, _ := strings.Cut(pair, "/")
		got, err := chromePlatform(goos, goarch)
		if err != nil || got != want {
			t.Errorf("%s: %q, %v; want %q", pair, got, err, want)
		}
	}
	if _, err := chromePlatform("plan9", "386"); err == nil || !strings.Contains(err.Error(), "DUMMYRUN_CHROMEDRIVER") {
		t.Errorf("plan9: %v, want an error that names DUMMYRUN_CHROMEDRIVER", err)
	}
}

// fakeInstall writes a complete-looking install for the current platform.
func fakeInstall(t *testing.T, root, version string) (chrome, driver string) {
	t.Helper()
	platform, err := ChromePlatform()
	if err != nil {
		t.Skip(err)
	}
	dir := filepath.Join(root, version)
	chrome, driver = chromeBinary(dir, platform), driverBinary(dir, platform)
	for _, p := range []string{chrome, driver, filepath.Join(dir, marker)} {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return chrome, driver
}

func TestCachedChrome(t *testing.T) {
	root := t.TempDir()
	t.Setenv(CacheEnv, root)
	if _, _, ok := CachedChrome(); ok {
		t.Fatal("an empty cache reported a Chrome")
	}
	// A folder without the completion marker is a failed install and is ignored.
	chrome, _ := fakeInstall(t, root, "100.0.0.1")
	if err := os.Remove(filepath.Join(root, "100.0.0.1", marker)); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := CachedChrome(); ok {
		t.Fatalf("an install without %s was used: %s", marker, chrome)
	}
	oldChrome, oldDriver := fakeInstall(t, root, "101.0.0.1")
	gotC, gotD, ok := CachedChrome()
	if !ok || gotC != oldChrome || gotD != oldDriver {
		t.Fatalf("got %q %q %v, want the 101 pair", gotC, gotD, ok)
	}
	pinnedChrome, pinnedDriver := fakeInstall(t, root, Pins().ChromeForTesting)
	gotC, gotD, ok = CachedChrome()
	if !ok || gotC != pinnedChrome || gotD != pinnedDriver {
		t.Fatalf("got %q %q, want the pinned pair to win", gotC, gotD)
	}
}

func TestCacheRootHonoursEnv(t *testing.T) {
	t.Setenv(CacheEnv, "/somewhere")
	if got, _ := CacheRoot(); got != "/somewhere" {
		t.Errorf("CacheRoot() = %q", got)
	}
	t.Setenv(CacheEnv, "")
	got, err := CacheRoot()
	if err == nil && !strings.HasSuffix(filepath.ToSlash(got), "dummyrun/browsers") {
		t.Errorf("CacheRoot() = %q, want it to end in dummyrun/browsers", got)
	}
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// fakeServer serves a chrome and a chromedriver archive for the current platform and returns matching pins.
func fakeServer(t *testing.T) (*httptest.Server, Versions) {
	t.Helper()
	platform, err := ChromePlatform()
	if err != nil {
		t.Skip(err)
	}
	rel := func(p string) string { r, _ := filepath.Rel("v", p); return filepath.ToSlash(r) }
	chromeZip := buildZip(t, map[string]string{rel(chromeBinary("v", platform)): "chrome"})
	driverZip := buildZip(t, map[string]string{rel(driverBinary("v", platform)): "driver"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/9.9.9.9/" + platform + "/chrome-" + platform + ".zip":
			_, _ = w.Write(chromeZip)
		case "/9.9.9.9/" + platform + "/chromedriver-" + platform + ".zip":
			_, _ = w.Write(driverZip)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	pins := Versions{ChromeForTesting: "9.9.9.9", Chrome: map[string]string{platform: sum(chromeZip)},
		Chromedriver: map[string]string{platform: sum(driverZip)}}
	return srv, pins
}

func TestInstallChrome(t *testing.T) {
	srv, pins := fakeServer(t)
	root := t.TempDir()
	var out bytes.Buffer
	o := Options{Root: root, ChromeBase: srv.URL, Pins: &pins, Out: &out}
	chrome, driver, err := InstallChrome(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{chrome, driver} {
		if !isFile(p) || !strings.HasPrefix(p, filepath.Join(root, "9.9.9.9")) {
			t.Errorf("%s is not an installed file in the cache", p)
		}
	}
	if !complete(filepath.Join(root, "9.9.9.9")) {
		t.Error("no completion marker")
	}
	if _, err := os.Stat(filepath.Join(root, "9.9.9.9.partial")); err == nil {
		t.Error("the staging folder was left behind")
	}
	// Run again: nothing is downloaded.
	srv.Close()
	out.Reset()
	if _, _, err := InstallChrome(context.Background(), o); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if !strings.Contains(out.String(), "already installed") {
		t.Errorf("second install said %q", out.String())
	}
}

func TestInstallChromeRejectsAWrongHash(t *testing.T) {
	srv, pins := fakeServer(t)
	platform, _ := ChromePlatform()
	pins.Chrome[platform] = strings.Repeat("0", 64)
	root := t.TempDir()
	_, _, err := InstallChrome(context.Background(), Options{Root: root, ChromeBase: srv.URL, Pins: &pins})
	if err == nil || !strings.Contains(err.Error(), "does not match its pinned SHA-256") {
		t.Fatalf("got %v, want a hash error", err)
	}
	if _, _, ok := cachedIn(t, root); ok {
		t.Error("a failed install was left usable")
	}
	if complete(filepath.Join(root, "9.9.9.9")) {
		t.Error("a failed install has a completion marker")
	}
}

func cachedIn(t *testing.T, root string) (string, string, bool) {
	t.Helper()
	t.Setenv(CacheEnv, root)
	return CachedChrome()
}

func TestInstallChromeOfflineNamesTheCommand(t *testing.T) {
	srv, pins := fakeServer(t)
	url := srv.URL
	srv.Close()
	_, _, err := InstallChrome(context.Background(), Options{Root: t.TempDir(), ChromeBase: url, Pins: &pins})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"dummyrun browsers install", "DUMMYRUN_CHROMEDRIVER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestUnzipRefusesEscapes(t *testing.T) {
	dir := t.TempDir()
	for name, files := range map[string]map[string]string{
		"dot dot": {"../evil": "x"},
		"nested":  {"a/../../evil": "x"},
	} {
		zp := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".zip")
		if err := os.WriteFile(zp, buildZip(t, files), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := unzip(zp, filepath.Join(dir, "out-"+strings.ReplaceAll(name, " ", "-"))); err == nil {
			t.Errorf("%s: unzip accepted an entry outside the folder", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil")); err == nil {
		t.Error("a file was written outside the folder")
	}
}

func TestUnzipSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no symbolic links")
	}
	mk := func(target string) string {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		h := &zip.FileHeader{Name: "app/link"}
		h.SetMode(os.ModeSymlink | 0o755)
		w, _ := zw.CreateHeader(h)
		_, _ = w.Write([]byte(target))
		_ = zw.Close()
		zp := filepath.Join(t.TempDir(), "a.zip")
		if err := os.WriteFile(zp, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		return zp
	}
	if err := unzip(mk("../../outside"), t.TempDir()); err == nil {
		t.Error("a link out of the folder was accepted")
	}
	dest := t.TempDir()
	if err := unzip(mk("sibling"), dest); err != nil {
		t.Fatalf("a link inside the folder was refused: %v", err)
	}
	if got, err := os.Readlink(filepath.Join(dest, "app", "link")); err != nil || got != "sibling" {
		t.Errorf("link %q, %v", got, err)
	}
}

// TestInstallIntegration downloads the real pinned Chrome for Testing and Firefox and starts them. The
// files are large, so it runs only when DUMMYRUN_TEST_BROWSER_DOWNLOAD is set; the engine workflow sets it once.
func TestInstallIntegration(t *testing.T) {
	if os.Getenv("DUMMYRUN_TEST_BROWSER_DOWNLOAD") == "" {
		t.Skip("set DUMMYRUN_TEST_BROWSER_DOWNLOAD=1 to download the real browsers")
	}
	root := t.TempDir()
	o := Options{Root: root, Out: testWriter{t}}
	chrome, driver, err := InstallChrome(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][]string{{chrome, "--version"}, {driver, "--version"}} {
		out, err := exec.CommandContext(context.Background(), c[0], c[1:]...).CombinedOutput() //nolint:gosec // paths this test just installed
		if err != nil {
			t.Fatalf("%s: %v: %s", c[0], err, out)
		}
		if !strings.Contains(string(out), Pins().ChromeForTesting[:3]) {
			t.Errorf("%s printed %q, want the pinned major version", c[0], out)
		}
	}
	t.Setenv(CacheEnv, root)
	if c, d, ok := CachedChrome(); !ok || c != chrome || d != driver {
		t.Errorf("CachedChrome() = %q %q %v after install", c, d, ok)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(b []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(b)))
	return len(b), nil
}

func TestDownloadLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "2147483648")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, err := Options{}.download(context.Background(), srv.URL+"/big.zip", strings.Repeat("0", 64), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("got %v, want the size limit error", err)
	}
}

func TestMarkerErrorOnlyForReadProblems(t *testing.T) {
	if err := markerError(t.TempDir()); err != nil {
		t.Errorf("a missing marker is not an error: %v", err)
	}
}

func TestUnzipRefusesWritingThroughALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no symbolic links")
	}
	dest, outside := t.TempDir(), t.TempDir()
	// An earlier entry left a link that points out of the folder.
	if err := os.Symlink(outside, filepath.Join(dest, "ln")); err != nil {
		t.Fatal(err)
	}
	zp := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(zp, buildZip(t, map[string]string{"ln/evil.txt": "x"}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := unzip(zp, dest); err == nil {
		t.Error("an entry was written through a link that leaves the folder")
	}
	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); err == nil {
		t.Error("a file landed outside the folder")
	}
}

// zipEntry is one archive entry, in the order given, with the mode a real archive would carry.
type zipEntry struct {
	name, body string
	mode       os.FileMode
}

func buildOrderedZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(e.body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestUnzipResolvesTheParentOfALink: sub/x points at ../other, so a link written at sub/x/l really sits in other,
// one level higher than its name says. Its target ../../out looks inside the folder by name and is not.
func TestUnzipResolvesTheParentOfALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no symbolic links")
	}
	dest := filepath.Join(t.TempDir(), "dest")
	zp := filepath.Join(t.TempDir(), "a.zip")
	data := buildOrderedZip(t, []zipEntry{
		{"other/", "", os.ModeDir | 0o755},
		{"sub/", "", os.ModeDir | 0o755},
		{"sub/x", "../other", os.ModeSymlink | 0o755},
		{"sub/x/l", "../../out", os.ModeSymlink | 0o755},
	})
	if err := os.WriteFile(zp, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := unzip(zp, dest); err == nil {
		t.Fatal("a link whose target leaves the folder, once its parent is resolved, was accepted")
	}
	if _, err := os.Lstat(filepath.Join(dest, "other", "l")); err == nil {
		t.Error("the escaping link was created")
	}
}
