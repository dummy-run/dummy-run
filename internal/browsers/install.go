package browsers

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Default download locations. Options can replace them, which the tests do.
const (
	DefaultChromeBase  = "https://storage.googleapis.com/chrome-for-testing-public"
	DefaultFirefoxBase = "https://download-installer.cdn.mozilla.net/pub/firefox/releases"
)

// maxUnpacked bounds what one archive may write, so a corrupt or hostile file cannot fill the disk.
const maxUnpacked = 4 << 30

// Options configure an install. The zero value installs the pinned versions into the user's cache.
type Options struct {
	// Root is the cache directory. Empty means CacheRoot.
	Root string
	// Client downloads files. Empty means a client with no overall timeout, since the files are large.
	Client *http.Client
	// ChromeBase and FirefoxBase replace the download locations.
	ChromeBase, FirefoxBase string
	// Pins replaces the embedded versions and hashes.
	Pins *Versions
	// Out gets progress lines. Empty discards them.
	Out io.Writer
}

func (o Options) root() (string, error) {
	if o.Root != "" {
		return o.Root, nil
	}
	return CacheRoot()
}

func (o Options) pins() Versions {
	if o.Pins != nil {
		return *o.Pins
	}
	return Pins()
}

func (o Options) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return &http.Client{}
}

func (o Options) say(format string, a ...any) {
	if o.Out != nil {
		fmt.Fprintf(o.Out, format+"\n", a...)
	}
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// InstallChrome downloads the pinned Chrome for Testing and its chromedriver, checks both against the
// pinned SHA-256 hashes and unpacks them into the cache. It returns the two paths. An install that is
// already complete is left alone.
func InstallChrome(ctx context.Context, o Options) (chrome, driver string, err error) {
	root, err := o.root()
	if err != nil {
		return "", "", err
	}
	platform, err := ChromePlatform()
	if err != nil {
		return "", "", err
	}
	pins := o.pins()
	ver := pins.ChromeForTesting
	dir := filepath.Join(root, ver)
	if complete(dir) && isFile(chromeBinary(dir, platform)) && isFile(driverBinary(dir, platform)) {
		o.say("Chrome for Testing %s is already installed in %s", ver, dir)
		return chromeBinary(dir, platform), driverBinary(dir, platform), nil
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", "", err
	}
	staging := dir + ".partial"
	if err := os.RemoveAll(staging); err != nil { // a leftover from an interrupted install, inside our own cache
		return "", "", err
	}
	if err := os.MkdirAll(staging, 0o750); err != nil {
		return "", "", err
	}
	base := orDefault(o.ChromeBase, DefaultChromeBase)
	for _, part := range []struct{ name, hash string }{
		{"chrome", pins.Chrome[platform]},
		{"chromedriver", pins.Chromedriver[platform]},
	} {
		url := fmt.Sprintf("%s/%s/%s/%s-%s.zip", base, ver, platform, part.name, platform)
		if part.hash == "" {
			return "", "", fmt.Errorf("browsers: no pinned SHA-256 for %s on %s", part.name, platform)
		}
		o.say("Downloading %s %s for %s", part.name, ver, platform)
		if err := o.fetchAndUnzip(ctx, url, part.hash, root, staging); err != nil {
			return "", "", err
		}
	}
	if !isFile(chromeBinary(staging, platform)) || !isFile(driverBinary(staging, platform)) {
		return "", "", errors.New("browsers: the downloaded archives did not hold the Chrome and chromedriver programs; run the command again or report it")
	}
	if err := os.WriteFile(filepath.Join(staging, marker), []byte(ver+"\n"), 0o600); err != nil {
		return "", "", err
	}
	if err := os.RemoveAll(dir); err != nil { // an earlier install that never finished
		return "", "", err
	}
	if err := os.Rename(staging, dir); err != nil {
		return "", "", err
	}
	o.say("Installed Chrome for Testing %s in %s", ver, dir)
	return chromeBinary(dir, platform), driverBinary(dir, platform), nil
}

// SystemFirefox finds a Firefox that is already on the machine: DUMMYRUN_FIREFOX, the macOS application,
// or a program on the PATH. It returns "" when there is none.
func SystemFirefox() string {
	cands := []string{os.Getenv("DUMMYRUN_FIREFOX"), "/Applications/Firefox.app/Contents/MacOS/firefox"}
	for _, c := range cands {
		if c != "" && isFile(c) {
			return c
		}
	}
	for _, n := range []string{"firefox", "firefox-esr"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// FirefoxResult says where Firefox is and how it got there.
type FirefoxResult struct {
	Path string
	// System is true when the Firefox was already on the machine and nothing was downloaded.
	System bool
}

// InstallFirefox makes sure a Firefox is available. A Firefox that is already installed is used as it is.
// On Linux it otherwise downloads the pinned release. On macOS and Windows it explains where to get Firefox,
// since the packages there are installers rather than archives.
func InstallFirefox(ctx context.Context, o Options) (FirefoxResult, error) {
	root, err := o.root()
	if err != nil {
		return FirefoxResult{}, err
	}
	pins := o.pins()
	dir := filepath.Join(root, "firefox-"+pins.Firefox)
	if complete(dir) && isFile(firefoxBinary(dir)) {
		o.say("Firefox %s is already installed in %s", pins.Firefox, dir)
		return FirefoxResult{Path: firefoxBinary(dir)}, nil
	}
	if p := SystemFirefox(); p != "" {
		o.say("Using the Firefox already on this machine: %s", p)
		return FirefoxResult{Path: p, System: true}, nil
	}
	platform, ok := firefoxPlatform(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return FirefoxResult{}, errors.New("browsers: install Firefox from https://www.mozilla.org/firefox/new/ and Dummy Run will find it. " +
			"On macOS it looks in /Applications/Firefox.app; elsewhere put firefox on your PATH or set DUMMYRUN_FIREFOX")
	}
	hash := pins.FirefoxLinux[platform]
	if hash == "" {
		return FirefoxResult{}, fmt.Errorf("browsers: no pinned SHA-256 for Firefox on %s", platform)
	}
	if _, err := exec.LookPath("tar"); err != nil {
		return FirefoxResult{}, errors.New("browsers: unpacking Firefox needs the tar and xz programs; install them or install Firefox yourself")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return FirefoxResult{}, err
	}
	staging := dir + ".partial"
	if err := os.RemoveAll(staging); err != nil {
		return FirefoxResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(staging, "firefox"), 0o750); err != nil {
		return FirefoxResult{}, err
	}
	base := orDefault(o.FirefoxBase, DefaultFirefoxBase)
	url := fmt.Sprintf("%s/%s/%s/en-US/firefox-%s.tar.xz", base, pins.Firefox, platform, pins.Firefox)
	o.say("Downloading Firefox %s for %s", pins.Firefox, platform)
	tmp, err := o.download(ctx, url, hash, root)
	if err != nil {
		return FirefoxResult{}, err
	}
	defer os.Remove(tmp) //nolint:errcheck // our own download file in our own cache
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- fixed program, arguments are paths this tool made
	out, err := exec.CommandContext(ctx, "tar", "-xJf", tmp, "-C", filepath.Join(staging, "firefox"), "--strip-components=1").CombinedOutput() //nolint:gosec // see above
	if err != nil {
		return FirefoxResult{}, fmt.Errorf("browsers: unpacking Firefox: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if !isFile(firefoxBinary(staging)) {
		return FirefoxResult{}, errors.New("browsers: the Firefox archive did not hold a firefox program")
	}
	if err := os.WriteFile(filepath.Join(staging, marker), []byte(pins.Firefox+"\n"), 0o600); err != nil {
		return FirefoxResult{}, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return FirefoxResult{}, err
	}
	if err := os.Rename(staging, dir); err != nil {
		return FirefoxResult{}, err
	}
	o.say("Installed Firefox %s in %s", pins.Firefox, dir)
	return FirefoxResult{Path: firefoxBinary(dir)}, nil
}

// download fetches url into a new file in dir and checks its SHA-256. It returns the file's path, and
// removes the file itself when the check fails.
func (o Options) download(ctx context.Context, url, wantSHA, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := o.client().Do(req)
	if err != nil {
		return "", offline(url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", offline(url, fmt.Errorf("the server answered %s", resp.Status))
	}
	f, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	fail := func(err error) (string, error) {
		_ = f.Close()
		_ = os.Remove(name) // our own download file in our own cache
		return "", err
	}
	h := sha256.New()
	pr := &progress{total: resp.ContentLength, out: o.Out}
	if _, err := io.Copy(io.MultiWriter(f, h, pr), resp.Body); err != nil {
		return fail(offline(url, err))
	}
	if err := f.Close(); err != nil {
		return fail(err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
		_ = os.Remove(name)
		return "", fmt.Errorf("browsers: %s does not match its pinned SHA-256 (got %s, want %s). It was not unpacked. "+
			"Run the command again; if it keeps failing, the file changed upstream, so report it", url, got, wantSHA)
	}
	return name, nil
}

func offline(url string, err error) error {
	return fmt.Errorf("browsers: could not download %s: %w\n"+
		"If you are offline, run this again when you are connected: dummyrun browsers install\n"+
		"Or install the browser yourself and set DUMMYRUN_CHROME and DUMMYRUN_CHROMEDRIVER (or DUMMYRUN_FIREFOX)", url, err)
}

// progress prints a line at each tenth of a download of known size.
type progress struct {
	total, seen int64
	next        int64
	out         io.Writer
	last        time.Time
}

func (p *progress) Write(b []byte) (int, error) {
	p.seen += int64(len(b))
	if p.out == nil || p.total <= 0 {
		return len(b), nil
	}
	if pct := p.seen * 100 / p.total; pct >= p.next && time.Since(p.last) > time.Second {
		fmt.Fprintf(p.out, "  %d%% of %d MB\n", pct, p.total>>20)
		p.next = pct/25*25 + 25
		p.last = time.Now()
	}
	return len(b), nil
}

func (o Options) fetchAndUnzip(ctx context.Context, url, wantSHA, tmpDir, dest string) error {
	tmp, err := o.download(ctx, url, wantSHA, tmpDir)
	if err != nil {
		return err
	}
	defer os.Remove(tmp) //nolint:errcheck // our own download file in our own cache
	return unzip(tmp, dest)
}

// unzip unpacks an archive into dest. It refuses names that leave dest and symbolic links that point out of it.
func unzip(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("browsers: %w", err)
	}
	defer func() { _ = zr.Close() }()
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	inside := func(p string) bool {
		rel, err := filepath.Rel(absDest, p)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	var written int64
	for _, f := range zr.File {
		target := filepath.Join(absDest, filepath.FromSlash(f.Name)) //nolint:gosec // checked by inside below
		if !inside(target) {
			return fmt.Errorf("browsers: the archive has an entry outside its folder: %q", f.Name)
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			rc, err := f.Open()
			if err != nil {
				return err
			}
			link, err := io.ReadAll(io.LimitReader(rc, 4096))
			_ = rc.Close()
			if err != nil {
				return err
			}
			if !inside(filepath.Join(filepath.Dir(target), string(link))) {
				return fmt.Errorf("browsers: the archive has a link that points outside its folder: %q", f.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			if err := os.Symlink(string(link), target); err != nil {
				return err
			}
		default:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			n, err := copyEntry(f, target, mode.Perm(), maxUnpacked-written)
			if err != nil {
				return err
			}
			written += n
		}
	}
	return nil
}

func copyEntry(f *zip.File, target string, perm os.FileMode, limit int64) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm|0o200) //nolint:gosec // target checked by the caller
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > limit {
		err = errors.New("browsers: the archive unpacks to more than 4 GB")
	}
	return n, err
}
