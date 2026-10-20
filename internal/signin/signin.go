// Package signin signs a run in before the persona starts. The engine does it, never the persona, so
// the model never sees a credential. There are two ways: the login form named in the target file, and
// a saved session file of cookies. Credentials come from environment variables and are kept out of
// every record, log, screenshot and error.
package signin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/record"
)

// Methods in the run record.
const (
	MethodForm    = "form"
	MethodSession = "session"
)

// Creds are a user name and password read from the environment.
type Creds struct {
	Username, Password string
}

// Secrets lists the values that must never be written anywhere.
func (c Creds) Secrets() []string { return []string{c.Username, c.Password} }

// Plan is a resolved sign-in: which role, by which method, with what credentials.
type Plan struct {
	// Role is the role name as the target file spells it.
	Role   string
	Method string
	Creds  Creds
	// SessionPath is the session file for the session method.
	SessionPath string
}

// Secrets lists the values to keep out of output.
func (p *Plan) Secrets() []string {
	if p == nil {
		return nil
	}
	return p.Creds.Secrets()
}

// Resolve picks the method for a role and reads its credentials. A session file given on the
// command line wins over the form. The error names an environment variable and never its value.
func Resolve(t policy.Target, role, sessionFlag string, getenv func(string) string) (*Plan, error) {
	return resolve(t, role, sessionFlag, true, getenv)
}

// ResolveForm is Resolve for a role's form credentials only. A session file, from the flag or from
// the target file, is ignored. "dummyrun session save" uses it, since it makes the session file.
func ResolveForm(t policy.Target, role string, getenv func(string) string) (*Plan, error) {
	return resolve(t, role, "", false, getenv)
}

func resolve(t policy.Target, role, sessionFlag string, useSession bool, getenv func(string) string) (*Plan, error) {
	s := t.Signin
	if s == nil {
		return nil, errors.New("the target file has no signin block, so there is nothing to sign in with")
	}
	cred, name, ok := s.Role(role)
	if !ok && sessionFlag == "" {
		return nil, fmt.Errorf("the target file has no credentials for the role %q; it has %s", role, strings.Join(s.Roles(), ", "))
	}
	if !ok {
		name = role
	}
	switch {
	case sessionFlag != "":
		return &Plan{Role: name, Method: MethodSession, SessionPath: sessionFlag}, nil
	case useSession && cred.SessionFile != "":
		return &Plan{Role: name, Method: MethodSession, SessionPath: s.SessionPath(cred)}, nil
	}
	if cred.UsernameEnv == "" || cred.PasswordEnv == "" {
		return nil, fmt.Errorf("the role %s has no username_env and password_env, so it cannot sign in through the form", name)
	}
	user, pass := getenv(cred.UsernameEnv), getenv(cred.PasswordEnv)
	var missing []string
	if user == "" {
		missing = append(missing, cred.UsernameEnv)
	}
	if pass == "" {
		missing = append(missing, cred.PasswordEnv)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("sign-in as %s needs the environment variable %s, which is not set", name, strings.Join(missing, " and "))
	}
	return &Plan{Role: name, Method: MethodForm, Creds: Creds{Username: user, Password: pass}}, nil
}

// Scrub removes every secret from s. A secret shorter than three characters is skipped, since
// removing it would also mangle ordinary text.
func Scrub(s string, secrets ...string) string {
	for _, sec := range secrets {
		if len(sec) >= 3 {
			s = strings.ReplaceAll(s, sec, "[hidden]")
		}
	}
	return s
}

// scrubbedError shows a message with the secrets removed and still unwraps to the original error,
// so errors.Is can tell a cancelled sign-in from a failed one.
type scrubbedError struct {
	msg   string
	cause error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.cause }

// ScrubError returns an error whose message has the secrets removed. It unwraps to err.
func ScrubError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	return &scrubbedError{msg: Scrub(err.Error(), secrets...), cause: err}
}

// Hook is the request policy for a run that signs in without a persona: the target's normal
// policy plus the gate's one exception. The runner builds its own hook, which also adds telemetry
// headers, from the same two parts.
func Hook(t policy.Target, g *policy.Gate) func(page.Request) page.Decision {
	return func(req page.Request) page.Decision {
		if g.Take(req.Method, req.URL) {
			return page.Decision{}
		}
		if v := t.Decide(req.Method, req.URL); v != policy.Allow {
			return page.Decision{Block: true, Reason: v.String()}
		}
		return page.Decision{}
	}
}

// Do signs the page's browser in as the plan says and returns the block for the run record. The
// gate is open for the form method only, and closed again before Do returns. A non-nil error is
// also described in the returned block, with credentials removed.
func Do(ctx context.Context, p *page.Page, t policy.Target, plan *Plan, g *policy.Gate) (*record.Signin, error) {
	started := time.Now()
	res := &record.Signin{Method: plan.Method, Role: plan.Role}
	var err error
	switch plan.Method {
	case MethodForm:
		g.Open()
		err = submitForm(ctx, p, t, plan.Creds, g)
		g.Close()
	case MethodSession:
		err = loadSession(ctx, p, t, plan.SessionPath)
	default:
		err = fmt.Errorf("unknown sign-in method %q", plan.Method)
	}
	if m, u, ok := g.Used(); ok {
		res.AllowedRequest = &record.SigninRequest{Method: m, URL: u}
	}
	res.DurationMs = int(time.Since(started).Milliseconds())
	if err != nil {
		res.Error = Scrub(err.Error(), plan.Secrets()...)
		return res, &scrubbedError{msg: fmt.Sprintf("sign in as %s: %s", plan.Role, res.Error), cause: err}
	}
	res.OK = true
	return res, nil
}
