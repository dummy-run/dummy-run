package a11y

import (
	"context"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/signin"
)

// fakeConn stands in for the browser connection. It counts what the session load asks of it.
type fakeConn struct{ cookies, navigations int }

func (f *fakeConn) CreateContext(context.Context) (string, error) { return "tab", nil }
func (f *fakeConn) CloseContext(context.Context, string) error    { return nil }
func (f *fakeConn) SetCookie(context.Context, bidi.Cookie) error  { f.cookies++; return nil }
func (f *fakeConn) Navigate(context.Context, string, string) (bidi.NavigateResult, error) {
	f.navigations++
	return bidi.NavigateResult{}, nil
}
func (f *fakeConn) CallFunctionJSON(context.Context, string, string, any, ...any) error { return nil }

func TestApplySessionLoadsCookiesForTheSweepHosts(t *testing.T) {
	conn := &fakeConn{}
	cfg := Config{URLs: []string{"https://app.test/page"}, Session: &signin.Session{
		Cookies: []signin.SessionCookie{{Name: "sid", Domain: "app.test", Expires: -1}},
	}}
	if err := applySession(context.Background(), conn, cfg); err != nil {
		t.Fatal(err)
	}
	if conn.cookies != 1 {
		t.Errorf("%d cookies set, want 1", conn.cookies)
	}
}

func TestApplySessionRefusesASessionThatLoadsNothing(t *testing.T) {
	conn := &fakeConn{}
	cfg := Config{URLs: []string{"https://app.test/page"}, Session: &signin.Session{
		Cookies: []signin.SessionCookie{{Name: "sid", Domain: "other.test", Expires: -1}},
		Origins: []signin.SessionOrigin{{Origin: "https://other.test", LocalStorage: []signin.StorageItem{{Name: "a", Value: "1"}}}},
	}}
	err := applySession(context.Background(), conn, cfg)
	if err == nil || !strings.Contains(err.Error(), "no cookie or local storage") {
		t.Fatalf("got %v, want an error before the sweep visits any page", err)
	}
	if conn.cookies != 0 || conn.navigations != 0 {
		t.Errorf("%d cookies and %d visits, want none", conn.cookies, conn.navigations)
	}
}
