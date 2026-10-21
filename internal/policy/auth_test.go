package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

func targetJSON(auth string) string {
	return `{"environment":"preview","base_url":"https://preview.example.com/","allowed_hosts":["preview.example.com"],"auth":` + auth + `}`
}

func TestAuthParses(t *testing.T) {
	var tg Target
	err := json.Unmarshal([]byte(targetJSON(`{"basic":{"username_env":"PREVIEW_USER","password_env":"PREVIEW_PASS"},
		"headers":{"x-vercel-protection-bypass":{"value_env":"VERCEL_BYPASS"},"CF-Access-Client-Id":{"value_env":"CF_ID"}}}`)), &tg)
	if err != nil {
		t.Fatal(err)
	}
	if err := tg.Validate(); err != nil {
		t.Fatal(err)
	}
	if tg.Auth.Basic.PasswordEnv != "PREVIEW_PASS" || tg.Auth.Headers["CF-Access-Client-Id"].ValueEnv != "CF_ID" {
		t.Errorf("auth %+v", tg.Auth)
	}
	if got := strings.Join(tg.Auth.HeaderNames(), ","); got != "CF-Access-Client-Id,x-vercel-protection-bypass" {
		t.Errorf("header names %s", got)
	}
}

// A secret written into the file is refused, and the error never repeats it.
func TestAuthRejectsLiteralSecrets(t *testing.T) {
	const secret = "hunter2-not-a-real-secret"
	cases := map[string]string{
		"basic password":    `{"basic":{"username_env":"U","password":"` + secret + `"}}`,
		"basic username":    `{"basic":{"username":"` + secret + `","password_env":"P"}}`,
		"header value":      `{"headers":{"X-Token":{"value":"` + secret + `"}}}`,
		"header bare":       `{"headers":{"X-Token":"` + secret + `"}}`,
		"top-level token":   `{"token":"` + secret + `"}`,
		"secret as env":     `{"headers":{"X-Token":{"value_env":"` + secret + `"}}}`,
		"password as env":   `{"basic":{"username_env":"U","password_env":"` + secret + `"}}`,
		"unknown field":     `{"headers":{"X-Token":{"value_env":"T","extra":"` + secret + `"}}}`,
		"bearer shorthand":  `{"bearer":"` + secret + `"}`,
		"empty":             `{}`,
		"missing password":  `{"basic":{"username_env":"U"}}`,
		"forbidden header":  `{"headers":{"Host":{"value_env":"T"}}}`,
		"bad header name":   `{"headers":{"Bad Name":{"value_env":"T"}}}`,
		"duplicate by case": `{"headers":{"X-A":{"value_env":"T"},"x-a":{"value_env":"T"}}}`,
		"clashing auth":     `{"basic":{"username_env":"U","password_env":"P"},"headers":{"Authorization":{"value_env":"T"}}}`,
	}
	for name, auth := range cases {
		t.Run(name, func(t *testing.T) {
			var tg Target
			err := json.Unmarshal([]byte(targetJSON(auth)), &tg)
			if err == nil {
				err = tg.Validate()
			}
			if err == nil {
				t.Fatal("accepted")
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("the error repeats the secret: %v", err)
			}
		})
	}
}

func TestAuthLiteralErrorIsClear(t *testing.T) {
	var tg Target
	err := json.Unmarshal([]byte(targetJSON(`{"basic":{"username_env":"U","password":"x"}}`)), &tg)
	if err == nil || !strings.Contains(err.Error(), "environment variable") || !strings.Contains(err.Error(), "password_env") {
		t.Errorf("error %v", err)
	}
}

func TestNoAuthIsFine(t *testing.T) {
	var tg Target
	if err := json.Unmarshal([]byte(`{"environment":"p","base_url":"https://a.example/","allowed_hosts":["a.example"]}`), &tg); err != nil {
		t.Fatal(err)
	}
	if err := tg.Validate(); err != nil || tg.Auth != nil {
		t.Errorf("err %v auth %v", err, tg.Auth)
	}
}
