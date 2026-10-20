package mcpserver

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestOriginCheck(t *testing.T) {
	o, err := ParseOrigins([]string{"preview.example.com", "*.vercel-previews.dev", "https://only-https.example.org:8443"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		url  string
		want bool
	}{
		{"http://localhost:3000/", true},
		{"http://127.0.0.1:8080/x", true},
		{"http://[::1]:3000/", true},
		{"http://192.168.1.20:3000/", true},
		{"http://10.0.0.5/", true},
		{"http://app.localhost/", true},
		{"http://staging.internal/", true},
		{"https://preview.example.com/a", true},
		{"https://pr-4.vercel-previews.dev/", true},
		{"https://only-https.example.org:8443/", true},
		{"http://only-https.example.org:8443/", false},
		{"https://only-https.example.org/", false},
		{"https://example.com/", false},
		{"https://evil-preview.example.com/", false},
		{"https://vercel-previews.dev/", false},
		{"http://169.254.169.254/latest/meta-data/", false},
		{"http://8.8.8.8/", false},
		{"http://[fd00:ec2::254]/latest/meta-data/", false},
		{"http://169.254.170.2/v2/credentials", false},
		{"http://[fd12:3456::1]/", true},
		{"http://metadata.google.internal/computeMetadata/v1/", false},
		{"http://instance-data.ec2.internal/latest/", false},
		{"http://METADATA.GOOGLE.INTERNAL./", false},
		{"http://localhost.evil.com/", false},
		{"file:///etc/passwd", false},
		{"javascript:alert(1)", false},
		{"http://user:pass@localhost/", false},
		{"localhost:3000", false},
		{"", false},
	} {
		_, err := o.Check(c.url)
		if (err == nil) != c.want {
			t.Errorf("Check(%q) error = %v, want allowed %v", c.url, err, c.want)
		}
	}
}

func TestOriginErrorSaysHowToAllow(t *testing.T) {
	_, err := (&Origins{}).Check("https://example.com/")
	if err == nil || !strings.Contains(err.Error(), "--allow-origin example.com") {
		t.Fatalf("got %v", err)
	}
}

func TestParseOriginsRejectsBroadEntries(t *testing.T) {
	for _, bad := range []string{"*", "*.com", "ftp://x.example.com", "https://x.example.com/path", "a*b.example.com", "user@x.example.com"} {
		if _, err := ParseOrigins([]string{bad}); err == nil {
			t.Errorf("ParseOrigins(%q) accepted it", bad)
		}
	}
}

func TestListedMetadataAddressesAreStillRefused(t *testing.T) {
	o, err := ParseOrigins([]string{"169.254.169.254", "100.100.100.200", "169.254.1.1", "192.0.0.192"})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"http://169.254.169.254/", "http://100.100.100.200/", "http://169.254.1.1/", "http://192.0.0.192/"} {
		if _, err := o.Check(u); err == nil {
			t.Errorf("%s was allowed although it is a metadata or link-local address", u)
		}
	}
}

func TestAPrivateNameThatResolvesToAMetadataAddressIsRefused(t *testing.T) {
	old := lookupIP
	defer func() { lookupIP = old }()
	lookupIP = func(_ context.Context, host string) ([]net.IPAddr, error) {
		switch host {
		case "app.internal":
			return []net.IPAddr{{IP: net.ParseIP("fd00:ec2::254")}}, nil
		case "link.test":
			return []net.IPAddr{{IP: net.ParseIP("169.254.1.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.5")}}, nil
	}
	o, _ := ParseOrigins(nil)
	for _, u := range []string{"https://app.internal/", "https://link.test/"} {
		if _, err := o.Check(u); err == nil {
			t.Errorf("%s was allowed", u)
		}
	}
	if _, err := o.Check("https://ok.internal/"); err != nil {
		t.Errorf("a private name that resolves to a private address was refused: %v", err)
	}
}
