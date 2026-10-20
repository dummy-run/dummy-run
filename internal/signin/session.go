package signin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
)

// Session is a saved sign-in: cookies and optionally local storage. The file format is the one
// Playwright calls storageState, so a file from either tool loads in the other.
type Session struct {
	Cookies []SessionCookie `json:"cookies"`
	Origins []SessionOrigin `json:"origins"`
}

// SessionCookie is one cookie. Expires is in seconds since the epoch, and -1 marks a session cookie.
type SessionCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	// SameSite is Strict, Lax or None.
	SameSite string `json:"sameSite"`
}

// SessionOrigin holds the local storage of one origin.
type SessionOrigin struct {
	Origin       string        `json:"origin"`
	LocalStorage []StorageItem `json:"localStorage"`
}

// StorageItem is one local storage entry.
type StorageItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// maxSessionBytes bounds a session file, so a wrong path cannot exhaust memory.
const maxSessionBytes = 8 << 20

// LoadSession reads a session file.
func LoadSession(path string) (*Session, error) {
	f, err := os.Open(path) //nolint:gosec // the path comes from the target file or the command line on purpose
	if err != nil {
		return nil, fmt.Errorf("session file: %w", err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > maxSessionBytes {
		return nil, fmt.Errorf("session file %s is larger than %d bytes", path, maxSessionBytes)
	}
	var s Session
	dec := json.NewDecoder(f)
	if err := dec.Decode(&s); err != nil {
		// A decode error can quote part of the file, which holds cookie values.
		return nil, fmt.Errorf("session file %s is not valid storageState JSON", path)
	}
	if len(s.Cookies) == 0 && len(s.Origins) == 0 {
		return nil, fmt.Errorf("session file %s holds no cookies and no local storage", path)
	}
	return &s, nil
}

// Save writes the session with mode 0600, through a temporary file in the same directory.
func (s *Session) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".session-*.tmp") // created with mode 0600
	if err != nil {
		return err
	}
	// The temporary file holds live cookies, so it must not stay behind when a step fails.
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		return fail(err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fail(err)
	}
	return nil
}

func (c SessionCookie) toBiDi() bidi.Cookie {
	ck := bidi.Cookie{Name: c.Name, Value: bidi.BytesValue{Type: "string", Value: c.Value}, Domain: c.Domain,
		Path: c.Path, HTTPOnly: c.HTTPOnly, Secure: c.Secure}
	if ck.Path == "" {
		ck.Path = "/"
	}
	switch strings.ToLower(c.SameSite) {
	case "strict":
		ck.SameSite = "strict"
	case "none":
		ck.SameSite = "none"
	default:
		ck.SameSite = "lax"
	}
	if c.Expires > 0 {
		e := int64(c.Expires)
		ck.Expiry = &e
	}
	return ck
}

func fromBiDi(ck bidi.Cookie) SessionCookie {
	c := SessionCookie{Name: ck.Name, Value: ck.Value.Value, Domain: ck.Domain, Path: ck.Path,
		HTTPOnly: ck.HTTPOnly, Secure: ck.Secure, Expires: -1}
	switch ck.SameSite {
	case "strict":
		c.SameSite = "Strict"
	case "none":
		c.SameSite = "None"
	default:
		c.SameSite = "Lax"
	}
	if ck.Expiry != nil {
		c.Expires = float64(*ck.Expiry)
	}
	return c
}

// StorageConn is the part of the browser connection that Apply uses. *bidi.Conn provides it, and a
// test can replace it.
type StorageConn interface {
	CreateContext(ctx context.Context) (string, error)
	CloseContext(ctx context.Context, context string) error
	SetCookie(ctx context.Context, ck bidi.Cookie) error
	Navigate(ctx context.Context, context, url string) (bidi.NavigateResult, error)
	CallFunctionJSON(ctx context.Context, context, declaration string, out any, args ...any) error
}

// Applied says what Apply did.
type Applied struct {
	Cookies int
	// Storage counts the local storage entries set.
	Storage int
	// Skipped counts cookies and storage entries left out: expired, or for a host the run may not reach.
	Skipped int
}

// Apply loads the session into the browser: cookies through the storage module, then local
// storage through a short visit to each origin. It uses a tab of its own and closes it. Entries
// for hosts that hostOK refuses are skipped, so a session file cannot plant anything outside the
// target. Expired cookies are skipped too.
func (s *Session) Apply(ctx context.Context, conn StorageConn, hostOK func(host string) bool, now time.Time) (Applied, error) {
	var out Applied
	cx, err := conn.CreateContext(ctx)
	if err != nil {
		return out, fmt.Errorf("open a tab: %w", err)
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.CloseContext(cctx, cx)
	}()
	for _, c := range s.Cookies {
		host := strings.TrimPrefix(c.Domain, ".")
		if !hostOK(host) || (c.Expires > 0 && time.Unix(int64(c.Expires), 0).Before(now)) {
			out.Skipped++
			continue
		}
		if err := conn.SetCookie(ctx, c.toBiDi()); err != nil {
			return out, fmt.Errorf("set the cookie %q: %w", c.Name, err)
		}
		out.Cookies++
	}
	for _, o := range s.Origins {
		u, err := url.Parse(o.Origin)
		if err != nil || u.Host == "" || !hostOK(u.Hostname()) {
			out.Skipped += len(o.LocalStorage)
			continue
		}
		if len(o.LocalStorage) == 0 {
			continue
		}
		nctx, cancel := context.WithTimeout(ctx, formTimeout)
		_, err = conn.Navigate(nctx, cx, o.Origin+"/")
		cancel()
		if err != nil {
			return out, fmt.Errorf("visit %s to set local storage: %w", o.Origin, err)
		}
		items, _ := json.Marshal(o.LocalStorage)
		var n int
		if err := conn.CallFunctionJSON(ctx, cx, setStorageJS, &n, string(items)); err != nil {
			return out, fmt.Errorf("set local storage for %s: %w", o.Origin, err)
		}
		out.Storage += len(o.LocalStorage)
	}
	return out, nil
}

const setStorageJS = `(itemsJSON) => {
  const items = JSON.parse(itemsJSON);
  for (const it of items) localStorage.setItem(it.name, it.value);
  return items.length;
}`

const readStorageJS = `() => ({ origin: location.origin, items: Object.keys(localStorage).map((k) => ({ name: k, value: localStorage.getItem(k) })) })`

// Capture reads the browser's cookies, and the local storage of the target's origin, into a session.
func Capture(ctx context.Context, p *page.Page, t policy.Target) (*Session, error) {
	conn := p.Conn()
	cks, err := conn.GetCookies(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the cookies: %w", err)
	}
	s := &Session{Cookies: []SessionCookie{}, Origins: []SessionOrigin{}}
	for _, ck := range cks {
		if t.HostAllowed(strings.TrimPrefix(ck.Domain, ".")) {
			s.Cookies = append(s.Cookies, fromBiDi(ck))
		}
	}
	cx, err := conn.CreateContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("open a tab: %w", err)
	}
	defer closeTab(p, cx)
	// Local storage belongs to an origin. Read it from the target's origin and from the sign-in page's
	// origin, which can be another allowed host.
	visit := []string{t.BaseURL}
	if t.Signin != nil {
		if login := t.LoginURL(); login != "" {
			visit = append(visit, login)
		}
	}
	seen := map[string]bool{}
	for _, addr := range visit {
		nctx, cancel := context.WithTimeout(ctx, formTimeout)
		_, err := conn.Navigate(nctx, cx, addr)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("visit %s to read local storage: %w", addr, err)
		}
		var ls struct {
			Origin string        `json:"origin"`
			Items  []StorageItem `json:"items"`
		}
		if err := conn.CallFunctionJSON(ctx, cx, readStorageJS, &ls); err != nil {
			return nil, fmt.Errorf("read local storage: %w", err)
		}
		if len(ls.Items) > 0 && !seen[ls.Origin] {
			seen[ls.Origin] = true
			s.Origins = append(s.Origins, SessionOrigin{Origin: ls.Origin, LocalStorage: ls.Items})
		}
	}
	if len(s.Cookies) == 0 && len(s.Origins) == 0 {
		return nil, errors.New("the browser holds no cookies or local storage after sign-in, so there is nothing to save")
	}
	return s, nil
}

// loadSession is the session method of Do.
func loadSession(ctx context.Context, p *page.Page, t policy.Target, path string) error {
	s, err := LoadSession(path)
	if err != nil {
		return err
	}
	a, err := s.Apply(ctx, p.Conn(), t.HostAllowed, time.Now())
	if err != nil {
		return err
	}
	if a.Cookies == 0 && a.Storage == 0 {
		return fmt.Errorf("nothing in the session file can be used: its cookies and local storage are expired or for hosts outside allowed_hosts")
	}
	return nil
}

// SaveForm signs in through the form in a new browser, using the same policy as a run, and
// writes the session to path. It returns how many cookies it saved.
func SaveForm(ctx context.Context, p *page.Page, t policy.Target, plan *Plan, g *policy.Gate, path string) (int, error) {
	if plan.Method != MethodForm {
		return 0, errors.New("session save signs in through the form, so the role needs username_env and password_env")
	}
	if _, err := Do(ctx, p, t, plan, g); err != nil {
		return 0, err
	}
	s, err := Capture(ctx, p, t)
	if err != nil {
		return 0, err
	}
	if err := s.Save(path); err != nil {
		return 0, err
	}
	return len(s.Cookies), nil
}
