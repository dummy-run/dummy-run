package signin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// fakeConn records what Apply asks the browser to do.
type fakeConn struct {
	cookies   []string
	visited   []string
	storageIn int
}

func (f *fakeConn) CreateContext(context.Context) (string, error) { return "tab", nil }
func (f *fakeConn) CloseContext(context.Context, string) error    { return nil }
func (f *fakeConn) SetCookie(_ context.Context, ck bidi.Cookie) error {
	f.cookies = append(f.cookies, ck.Name)
	return nil
}
func (f *fakeConn) Navigate(_ context.Context, _, url string) (bidi.NavigateResult, error) {
	f.visited = append(f.visited, url)
	return bidi.NavigateResult{}, nil
}
func (f *fakeConn) CallFunctionJSON(context.Context, string, string, any, ...any) error {
	f.storageIn++
	return nil
}

func TestApplyKeepsOnlyUsableEntries(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	s := &Session{
		Cookies: []SessionCookie{
			{Name: "good", Domain: "app.test", Expires: float64(now.Add(time.Hour).Unix())},
			{Name: "session-cookie", Domain: ".app.test", Expires: -1},
			{Name: "expired", Domain: "app.test", Expires: float64(now.Add(-time.Hour).Unix())},
			{Name: "elsewhere", Domain: "other.test", Expires: -1},
		},
		Origins: []SessionOrigin{
			{Origin: "https://app.test", LocalStorage: []StorageItem{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}}},
			{Origin: "https://other.test", LocalStorage: []StorageItem{{Name: "c", Value: "3"}}},
		},
	}
	conn := &fakeConn{}
	got, err := s.Apply(context.Background(), conn, func(h string) bool { return h == "app.test" }, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(conn.cookies, ",") != "good,session-cookie" {
		t.Errorf("cookies set: %v", conn.cookies)
	}
	if got.Cookies != 2 || got.Storage != 2 || got.Skipped != 3 {
		t.Errorf("applied %+v, want 2 cookies, 2 storage entries and 3 skipped (one expired, one other host, one other-origin entry)", got)
	}
	if len(conn.visited) != 1 || conn.visited[0] != "https://app.test/" {
		t.Errorf("visited %v, want only the allowed origin", conn.visited)
	}
}

func TestApplyCountsNothingForAnOriginOutsideTheTarget(t *testing.T) {
	s := &Session{Origins: []SessionOrigin{{Origin: "https://other.test", LocalStorage: []StorageItem{{Name: "c", Value: "3"}}}}}
	got, err := s.Apply(context.Background(), &fakeConn{}, func(string) bool { return false }, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Cookies != 0 || got.Storage != 0 {
		t.Errorf("applied %+v, want nothing, so a caller can refuse a session that signs nobody in", got)
	}
}

func TestSaveLeavesNoTemporaryFileWhenItFails(t *testing.T) {
	dir := t.TempDir()
	// The target path is a directory, so the final rename fails after the temporary file is written.
	target := filepath.Join(dir, "session.json")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Session{Cookies: []SessionCookie{{Name: "a", Value: "secret", Domain: "app.test"}}}
	if err := s.Save(target); err == nil {
		t.Fatal("want an error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".session-") {
			t.Errorf("temporary file %s was left behind", e.Name())
		}
	}
}

func TestScrubErrorKeepsTheCause(t *testing.T) {
	err := ScrubError(errors.New("password hunter22 failed: "+context.Canceled.Error()), "hunter22")
	if strings.Contains(err.Error(), "hunter22") {
		t.Errorf("message %q still holds the secret", err)
	}
	wrapped := ScrubError(context.Canceled, "hunter22")
	if !errors.Is(wrapped, context.Canceled) {
		t.Error("the scrubbed error does not unwrap to the cancellation")
	}
}

func TestApplySkipsStorageForAPlainHTTPOrigin(t *testing.T) {
	s := &Session{Origins: []SessionOrigin{
		{Origin: "http://app.test", LocalStorage: []StorageItem{{Name: "a", Value: "1"}}},
		{Origin: "https://app.test", LocalStorage: []StorageItem{{Name: "b", Value: "2"}}},
	}}
	conn := &fakeConn{}
	got, err := s.Apply(context.Background(), conn, func(string) bool { return true }, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Storage != 1 || len(conn.visited) != 1 || conn.visited[0] != "https://app.test/" {
		t.Errorf("applied %+v and visited %v, want only the https origin", got, conn.visited)
	}
}

func TestSecureAddress(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://app.test/login": true, "http://app.test/login": false,
		"http://localhost:3000/": true, "http://127.0.0.1:3000/": true, "ftp://app.test": false,
		"http://LOCALHOST/": true, "http://[::1]:3000/": true, "ftp://localhost/": false, "HTTPS://app.test/": true,
	} {
		if got := secureAddress(raw); got != want {
			t.Errorf("secureAddress(%q) = %v, want %v", raw, got, want)
		}
	}
}
