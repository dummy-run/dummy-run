package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/dummy-run/dummy-run/internal/policy"
)

// authFlags are the one-off flags for a protected preview. Each names environment variables; none takes a secret.
type authFlags struct {
	basic   string
	headers headerFlags
}

// headerFlags collects every --header-env.
type headerFlags []string

func (h *headerFlags) String() string     { return "" }
func (h *headerFlags) Set(v string) error { *h = append(*h, v); return nil }

func (a *authFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&a.basic, "basic-auth-env", "", "answer HTTP Basic auth with the user name and password in these environment variables, as USER_ENV:PASS_ENV. Names only, never the secrets")
	fs.Var(&a.headers, "header-env", "send a request header whose value is in an environment variable, as 'Name:ENV' (repeat for more). Only the target's own hosts get it. Names only, never the value")
}

// auth merges the flags over base (the target file's block, possibly nil) and validates the result.
// Errors never repeat what was typed, since a mistake here may be a secret in the wrong place.
func (a *authFlags) auth(base *policy.Auth) (*policy.Auth, error) {
	if a.basic == "" && len(a.headers) == 0 {
		return base, nil
	}
	out := &policy.Auth{}
	if base != nil {
		out.Basic = base.Basic
		out.Headers = map[string]policy.HeaderSource{}
		for k, v := range base.Headers {
			out.Headers[k] = v
		}
	}
	if a.basic != "" {
		user, pass, ok := strings.Cut(a.basic, ":")
		if !ok || user == "" || pass == "" || strings.Contains(pass, ":") {
			return nil, errors.New("--basic-auth-env: want USER_ENV:PASS_ENV, the names of two environment variables")
		}
		out.Basic = &policy.BasicAuth{UsernameEnv: user, PasswordEnv: pass}
	}
	for _, spec := range a.headers {
		name, env, ok := strings.Cut(spec, ":")
		if !ok || name == "" || env == "" || strings.Contains(env, ":") {
			return nil, errors.New("--header-env: want 'Name:ENV', a header name and the name of an environment variable")
		}
		if out.Headers == nil {
			out.Headers = map[string]policy.HeaderSource{}
		}
		for existing := range out.Headers {
			if strings.EqualFold(existing, name) {
				delete(out.Headers, existing)
			}
		}
		out.Headers[name] = policy.HeaderSource{ValueEnv: env}
	}
	if err := out.Validate(); err != nil {
		return nil, fmt.Errorf("preview access flags: %w", err)
	}
	return out, nil
}
