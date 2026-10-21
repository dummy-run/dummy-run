package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Auth declares how the browser gets past the access control in front of a protected preview: HTTP
// Basic auth, request headers such as a Vercel protection bypass or Cloudflare Access service token, or
// both. The secrets are never in the file. Each entry names the environment variable that holds its
// value, and the engine reads the variable when the run starts.
//
// The credentials go only to the target's own hosts (allowed_hosts), never to another origin.
type Auth struct {
	// Basic answers HTTP Basic auth.
	Basic *BasicAuth `json:"basic,omitempty"`
	// Headers maps a header name to where its value comes from.
	Headers map[string]HeaderSource `json:"headers,omitempty"`
}

// BasicAuth names the environment variables that hold a Basic auth user name and password.
type BasicAuth struct {
	UsernameEnv string `json:"username_env"`
	PasswordEnv string `json:"password_env"`
}

// HeaderSource names the environment variable that holds one header's value.
type HeaderSource struct {
	ValueEnv string `json:"value_env"`
}

// literalKeys are the keys a person would use to write a secret straight into the file.
var literalKeys = map[string]bool{
	"username": true, "user": true, "password": true, "pass": true, "value": true,
	"token": true, "secret": true, "key": true, "bearer": true, "credentials": true,
}

// rejectKeys fails with a clear error on a key that would hold a literal secret, and on any other
// key outside allowed. It never prints a value.
func rejectKeys(where string, raw map[string]json.RawMessage, allowed ...string) error {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ok := false
		for _, a := range allowed {
			ok = ok || k == a
		}
		if ok {
			continue
		}
		if literalKeys[strings.ToLower(k)] {
			return fmt.Errorf("%s: %q would put a secret in the target file, which is not allowed; name an environment variable with %s instead", where, k, strings.Join(allowed, " or "))
		}
		return fmt.Errorf("%s: unknown field %q (allowed: %s)", where, k, strings.Join(allowed, ", "))
	}
	return nil
}

func decodeStrict(data []byte, where string, into any, allowed ...string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%s: must be an object", where)
	}
	if err := rejectKeys(where, raw, allowed...); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	return nil
}

// UnmarshalJSON reads the auth block and refuses literal secrets.
func (a *Auth) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("auth: must be an object")
	}
	if err := rejectKeys("auth", raw, "basic", "headers"); err != nil {
		return err
	}
	*a = Auth{}
	if b, ok := raw["basic"]; ok {
		a.Basic = new(BasicAuth)
		if err := decodeStrict(b, "auth.basic", a.Basic, "username_env", "password_env"); err != nil {
			return err
		}
	}
	if h, ok := raw["headers"]; ok {
		var names map[string]json.RawMessage
		if err := json.Unmarshal(h, &names); err != nil {
			return fmt.Errorf("auth.headers: must be an object mapping header names to {\"value_env\": ...}")
		}
		a.Headers = make(map[string]HeaderSource, len(names))
		for name, v := range names {
			var src HeaderSource
			// A bare string here is the easy mistake: the secret itself.
			if bytes.HasPrefix(bytes.TrimSpace(v), []byte(`"`)) {
				return fmt.Errorf("auth.headers: header %q must be an object like {\"value_env\": \"NAME\"}; a value written in the target file is not allowed", name)
			}
			if err := decodeStrict(v, fmt.Sprintf("auth.headers[%q]", name), &src, "value_env"); err != nil {
				return err
			}
			a.Headers[name] = src
		}
	}
	return nil
}

// forbiddenHeaders are set by the browser or change how the message is framed.
var forbiddenHeaders = map[string]bool{
	"host": true, "content-length": true, "transfer-encoding": true, "connection": true,
	"upgrade": true, "te": true, "trailer": true, "proxy-authorization": true,
}

// ValidateEnvName reports whether s is a plausible environment variable name. A string that is not
// one is probably a secret pasted by mistake, so it is never echoed.
func ValidateEnvName(s string) bool { return envName.MatchString(s) }

func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return true
}

// Validate checks the block. Error messages name fields, never values.
func (a *Auth) Validate() error {
	if a == nil {
		return nil
	}
	if a.Basic == nil && len(a.Headers) == 0 {
		return fmt.Errorf("auth: give basic, headers or both")
	}
	if b := a.Basic; b != nil {
		if b.UsernameEnv == "" || b.PasswordEnv == "" {
			return fmt.Errorf("auth.basic: username_env and password_env are both required")
		}
		for field, v := range map[string]string{"username_env": b.UsernameEnv, "password_env": b.PasswordEnv} {
			if !ValidateEnvName(v) {
				return fmt.Errorf("auth.basic.%s is not an environment variable name; it holds the name of a variable, never the secret itself", field)
			}
		}
	}
	seen := map[string]bool{}
	for _, name := range a.HeaderNames() {
		if !validHeaderName(name) {
			return fmt.Errorf("auth.headers: %q is not a valid header name", name)
		}
		low := strings.ToLower(name)
		if forbiddenHeaders[low] {
			return fmt.Errorf("auth.headers: %s cannot be set; the browser controls it", name)
		}
		if seen[low] {
			return fmt.Errorf("auth.headers: %s is listed twice (header names ignore case)", name)
		}
		seen[low] = true
		src := a.Headers[name]
		if src.ValueEnv == "" {
			return fmt.Errorf("auth.headers[%q]: value_env is required", name)
		}
		if !ValidateEnvName(src.ValueEnv) {
			return fmt.Errorf("auth.headers[%q].value_env is not an environment variable name; it holds the name of a variable, never the secret itself", name)
		}
	}
	if a.Basic != nil && seen["authorization"] {
		return fmt.Errorf("auth: basic and an Authorization header would clash; use one of them")
	}
	return nil
}

// HeaderNames lists the configured header names in sorted order.
func (a *Auth) HeaderNames() []string {
	if a == nil {
		return nil
	}
	out := make([]string, 0, len(a.Headers))
	for n := range a.Headers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
