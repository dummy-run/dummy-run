package policy

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Signin declares how the engine signs in before a persona starts. The engine does it, not the
// persona, so the model never sees the credentials. The credentials themselves are never in the
// file: each role names the environment variables that hold them.
type Signin struct {
	// LoginURL is the page with the sign-in form. A relative address is resolved against base_url.
	LoginURL string `json:"login_url"`
	// Username, Password and Submit find the form's controls.
	Username Field `json:"username"`
	Password Field `json:"password"`
	Submit   Field `json:"submit"`
	// AllowRequest is the one request the sign-in phase may send on a read-only target. It is the
	// form's own POST, matched by method and URL.
	AllowRequest AllowedRequest `json:"allow_request"`
	// Success says how the engine knows the sign-in worked.
	Success Success `json:"success"`
	// Credentials maps a role name to where its credentials come from.
	Credentials map[string]Credential `json:"credentials"`
	// BlockPaths are paths that sign the persona out, such as /logout. A request to one of them
	// fails whatever its method, so the persona cannot end its own session.
	BlockPaths []string `json:"block_paths,omitempty"`

	// Dir is the directory of the target file. A relative session_file is resolved against it.
	Dir string `json:"-"`
}

// Field finds one control by CSS selector or by its label text. Give one of the two.
type Field struct {
	Selector string `json:"selector,omitempty"`
	Label    string `json:"label,omitempty"`
}

// AllowedRequest is the sign-in request that the read-only policy lets through.
type AllowedRequest struct {
	// Method defaults to POST.
	Method string `json:"method,omitempty"`
	// URL is the address the form posts to. A relative address is resolved against base_url.
	// The query and fragment are ignored when requests are matched.
	URL string `json:"url"`
}

// Success is the check that a sign-in worked. When both fields are set both must hold.
type Success struct {
	// URLMatches is a regular expression that the page address must match.
	URLMatches string `json:"url_matches,omitempty"`
	// Selector is a CSS selector for an element that only a signed-in page has.
	Selector string `json:"selector,omitempty"`
}

// Credential is where one role's credentials come from.
type Credential struct {
	// UsernameEnv and PasswordEnv name environment variables. The values are read when the run starts.
	UsernameEnv string `json:"username_env,omitempty"`
	PasswordEnv string `json:"password_env,omitempty"`
	// SessionFile is a saved session (see "dummyrun session save"). When set it is loaded in place of the form.
	SessionFile string `json:"session_file,omitempty"`
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Role returns the credential for a role, matched case-insensitively.
func (s *Signin) Role(role string) (Credential, string, bool) {
	if s == nil {
		return Credential{}, "", false
	}
	for name, c := range s.Credentials {
		if strings.EqualFold(name, role) {
			return c, name, true
		}
	}
	return Credential{}, "", false
}

// Roles lists the role names in sorted order.
func (s *Signin) Roles() []string {
	var out []string
	for name := range s.Credentials {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// SessionPath resolves a credential's session file against the target file's directory.
func (s *Signin) SessionPath(c Credential) string {
	if c.SessionFile == "" || filepath.IsAbs(c.SessionFile) || s.Dir == "" {
		return c.SessionFile
	}
	return filepath.Join(s.Dir, c.SessionFile)
}

// method is the method of the allowed request, POST by default.
func (a AllowedRequest) method() string {
	if a.Method == "" {
		return "POST"
	}
	return strings.ToUpper(a.Method)
}

// resolve turns an address from the target file into an absolute URL on the target.
func (t Target) resolve(raw string) (*url.URL, error) {
	base, err := url.Parse(t.BaseURL)
	if err != nil {
		return nil, err
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(ref), nil
}

// LoginURL is the absolute address of the sign-in page.
func (t Target) LoginURL() string {
	if t.Signin == nil {
		return ""
	}
	u, err := t.resolve(t.Signin.LoginURL)
	if err != nil {
		return ""
	}
	return u.String()
}

// SigninRequest is the method and absolute URL of the one request the sign-in phase may send.
func (t Target) SigninRequest() (method, rawURL string) {
	if t.Signin == nil {
		return "", ""
	}
	u, err := t.resolve(t.Signin.AllowRequest.URL)
	if err != nil {
		return "", ""
	}
	u.RawQuery, u.Fragment = "", ""
	return t.Signin.AllowRequest.method(), u.String()
}

func (f Field) validate(name string) error {
	if (f.Selector == "") == (f.Label == "") {
		return fmt.Errorf("signin.%s: give exactly one of selector and label", name)
	}
	return nil
}

func (t Target) validateSignin() error {
	s := t.Signin
	if s.LoginURL == "" {
		return fmt.Errorf("signin.login_url is required")
	}
	for name, raw := range map[string]string{"login_url": s.LoginURL, "allow_request.url": s.AllowRequest.URL} {
		if raw == "" {
			return fmt.Errorf("signin.%s is required", name)
		}
		u, err := t.resolve(raw)
		if err != nil {
			return fmt.Errorf("signin.%s: %w", name, err)
		}
		if !t.HostAllowed(u.Hostname()) {
			return fmt.Errorf("signin.%s: host %s is not in allowed_hosts", name, u.Hostname())
		}
		// The password goes to these addresses, so they follow the same rule as base_url.
		if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
			return fmt.Errorf("signin.%s must use https (or localhost)", name)
		}
	}
	for name, f := range map[string]Field{"username": s.Username, "password": s.Password, "submit": s.Submit} {
		if err := f.validate(name); err != nil {
			return err
		}
	}
	switch s.AllowRequest.method() {
	case "GET", "HEAD", "OPTIONS":
		return fmt.Errorf("signin.allow_request.method %s needs no exception; the read-only policy already allows it", s.AllowRequest.method())
	}
	if s.Success.URLMatches == "" && s.Success.Selector == "" {
		return fmt.Errorf("signin.success needs url_matches or selector, so a failed sign-in is never mistaken for a good one")
	}
	if s.Success.URLMatches != "" {
		if _, err := regexp.Compile(s.Success.URLMatches); err != nil {
			return fmt.Errorf("signin.success.url_matches: %w", err)
		}
	}
	if len(s.Credentials) == 0 {
		return fmt.Errorf("signin.credentials must name at least one role")
	}
	for role, c := range s.Credentials {
		if role == "" {
			return fmt.Errorf("signin.credentials has an empty role name")
		}
		if c.SessionFile != "" {
			continue
		}
		if !envName.MatchString(c.UsernameEnv) || !envName.MatchString(c.PasswordEnv) {
			return fmt.Errorf("signin.credentials.%s: username_env and password_env must each name an environment variable (never the value itself)", role)
		}
	}
	for _, p := range s.BlockPaths {
		if !strings.HasPrefix(p, "/") {
			return fmt.Errorf("signin.block_paths: %q must start with /", p)
		}
	}
	return nil
}

// signoutBlocked reports whether the URL is one of the declared sign-out paths.
func (t Target) signoutBlocked(u *url.URL) bool {
	if t.Signin == nil {
		return false
	}
	p := strings.TrimSuffix(u.Path, "/")
	for _, b := range t.Signin.BlockPaths {
		if strings.EqualFold(strings.TrimSuffix(b, "/"), p) {
			return true
		}
	}
	return false
}

// Gate is the one exception to the read-only policy. It is open only while the engine signs in,
// and it lets exactly one request through: the sign-in form's own, by method and URL. Every other
// request, and any second request that matches, goes back to the normal policy.
type Gate struct {
	method, rawURL string

	mu    sync.Mutex
	open  bool
	taken bool
	// Method and URL of the request that was let through.
	gotMethod, gotURL string
}

// NewGate builds the gate for a target. A target with no sign-in gets a gate that never opens.
func NewGate(t Target) *Gate {
	m, u := t.SigninRequest()
	return &Gate{method: m, rawURL: u}
}

// Open starts the sign-in phase.
func (g *Gate) Open() {
	g.mu.Lock()
	g.open = true
	g.mu.Unlock()
}

// Close ends the sign-in phase. The gate stays closed for the rest of the run.
func (g *Gate) Close() {
	g.mu.Lock()
	g.open = false
	g.mu.Unlock()
}

// Take reports whether the request is the sign-in request and the gate is open and unused. It
// uses the gate up when it says yes.
func (g *Gate) Take(method, rawURL string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.open || g.taken || g.rawURL == "" || !strings.EqualFold(method, g.method) {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	u.RawQuery, u.Fragment = "", ""
	if u.String() != g.rawURL {
		return false
	}
	g.taken = true
	g.gotMethod, g.gotURL = strings.ToUpper(method), u.String()
	return true
}

// Used returns the method and URL of the request the gate let through, and whether it did.
func (g *Gate) Used() (method, rawURL string, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.gotMethod, g.gotURL, g.taken
}
