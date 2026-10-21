package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/policy"
)

func parseAuth(t *testing.T, args ...string) *authFlags {
	t.Helper()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var a authFlags
	a.register(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return &a
}

func TestAuthFlags(t *testing.T) {
	a := parseAuth(t, "--basic-auth-env", "U_ENV:P_ENV", "--header-env", "x-vercel-protection-bypass:BYPASS", "--header-env", "CF-Access-Client-Id:CF_ID")
	got, err := a.auth(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Basic.UsernameEnv != "U_ENV" || got.Basic.PasswordEnv != "P_ENV" || len(got.Headers) != 2 {
		t.Errorf("auth %+v", got)
	}
}

func TestAuthFlagsOverrideTheFile(t *testing.T) {
	base := &policy.Auth{Headers: map[string]policy.HeaderSource{"X-Token": {ValueEnv: "FROM_FILE"}, "Keep": {ValueEnv: "KEEP"}}}
	a := parseAuth(t, "--header-env", "x-token:FROM_FLAG")
	got, err := a.auth(base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Headers["x-token"].ValueEnv != "FROM_FLAG" || got.Headers["Keep"].ValueEnv != "KEEP" || len(got.Headers) != 2 {
		t.Errorf("headers %+v", got.Headers)
	}
	if len(base.Headers) != 2 || base.Headers["X-Token"].ValueEnv != "FROM_FILE" {
		t.Error("the file's block was changed")
	}
	none := parseAuth(t)
	if got, err := none.auth(base); err != nil || got != base {
		t.Error("no flags should leave the file's block alone")
	}
}

func TestAuthFlagErrorsDoNotEchoInput(t *testing.T) {
	const secret = "s3cret-typed-by-mistake"
	for _, args := range [][]string{
		{"--basic-auth-env", "user:" + secret + " with space"},
		{"--basic-auth-env", secret},
		{"--basic-auth-env", "a:b:" + secret},
		{"--header-env", secret},
		{"--header-env", "Name:" + secret + "!"},
		{"--header-env", "Host:HOSTVAR"},
	} {
		a := parseAuth(t, args...)
		_, err := a.auth(nil)
		if err == nil {
			t.Errorf("%v: accepted", args)
			continue
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("%v: the error repeats the input: %v", args, err)
		}
	}
}
