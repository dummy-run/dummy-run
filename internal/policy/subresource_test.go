package policy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeLookup answers from a table. A value may list several addresses separated by commas.
func fakeLookup(m map[string]string) LookupFunc {
	return func(_ context.Context, host string) ([]net.IP, error) {
		v, ok := m[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		var ips []net.IP
		for _, s := range strings.Split(v, ",") {
			ips = append(ips, net.ParseIP(s))
		}
		return ips, nil
	}
}

func TestSubresourcesMayComeFromPublicHosts(t *testing.T) {
	ro := target(true)
	ro.Lookup = fakeLookup(map[string]string{"ichef.cdn.test": "93.184.216.34", "rebind.test": "10.0.0.7", "meta.test": "169.254.169.254",
		"mixed.test": "93.184.216.34,10.0.0.7", "www.example.test": "93.184.216.35", "v6.cdn.test": "2606:2800:220:1:248:1893:25c8:1946"})
	cases := []struct {
		name   string
		method string
		url    string
		kind   Kind
		want   Verdict
	}{
		{"own host page", "GET", "https://www.example.test/a", Navigation, Allow},
		{"cdn image", "GET", "https://ichef.cdn.test/x.jpg", Subresource, Allow},
		{"cdn head", "HEAD", "https://ichef.cdn.test/x.jpg", Subresource, Allow},
		{"navigation to the cdn", "GET", "https://ichef.cdn.test/", Navigation, BlockHost},
		{"write to the cdn", "POST", "https://ichef.cdn.test/collect", Subresource, BlockHost},
		{"options to the cdn", "OPTIONS", "https://ichef.cdn.test/x", Subresource, BlockHost},
		{"write to own host", "POST", "https://www.example.test/subscribe", Subresource, BlockWrite},
		{"name that resolves inside", "GET", "https://rebind.test/x.png", Subresource, BlockHost},
		{"metadata address by name", "GET", "https://meta.test/latest", Subresource, BlockHost},
		{"metadata address", "GET", "http://169.254.169.254/latest", Subresource, BlockHost},
		{"loopback", "GET", "http://127.0.0.1:9000/x", Subresource, BlockHost},
		{"private literal", "GET", "https://192.168.1.4/x", Subresource, BlockHost},
		{"ipv6 loopback", "GET", "http://[::1]/x", Subresource, BlockHost},
		{"localhost", "GET", "http://localhost:3000/x", Subresource, BlockHost},
		{"internal suffix", "GET", "https://db.internal/x", Subresource, BlockHost},
		{"bare name", "GET", "https://intranet/x", Subresource, BlockHost},
		{"name that does not resolve", "GET", "https://nowhere.test/x", Subresource, BlockHost},
		{"not http", "GET", "ftp://ichef.cdn.test/x", Subresource, BlockHost},
		{"public literal", "GET", "https://93.184.216.34/x", Subresource, Allow},
		{"one public and one internal address", "GET", "https://mixed.test/x.png", Subresource, BlockHost},
		{"public ipv6 name", "GET", "https://v6.cdn.test/x.png", Subresource, Allow},
		{"carrier-grade nat", "GET", "http://100.64.0.1/x", Subresource, BlockHost},
		{"ipv4-mapped ipv6", "GET", "http://[::ffff:10.0.0.1]/x", Subresource, BlockHost},
		{"nat64 to a private address", "GET", "http://[64:ff9b::a00:1]/x", Subresource, BlockHost},
		{"6to4 to a private address", "GET", "http://[2002:a00:1::1]/x", Subresource, BlockHost},
		{"this network", "GET", "http://0.1.2.3/x", Subresource, BlockHost},
		{"benchmarking", "GET", "http://198.18.0.1/x", Subresource, BlockHost},
		{"reserved", "GET", "http://240.0.0.1/x", Subresource, BlockHost},
		{"ietf assignments", "GET", "http://192.0.0.9/x", Subresource, BlockHost},
		// The allowed host on another port is a different origin, but a public one, so its images may load.
		{"own public host on another port", "GET", "https://www.example.test:8443/x.png", Subresource, Allow},
		{"page on own host on another port", "GET", "https://www.example.test:8443/", Navigation, BlockHost},
	}
	for _, c := range cases {
		if got := ro.DecideRequest(c.method, c.url, c.kind); got != c.want {
			t.Errorf("%s: %s %s = %v, want %v", c.name, c.method, c.url, got, c.want)
		}
	}
}

// A local target cannot reach another service on the machine through a page's images or scripts. A second
// port the app needs goes in allowed_hosts.
func TestALocalTargetCannotReachOtherLocalServices(t *testing.T) {
	local := Target{Adapter: "url", Environment: "local", BaseURL: "http://127.0.0.1:8080/", AllowedHosts: []string{"127.0.0.1"}, ReadOnly: true}
	for _, u := range []string{"http://localhost:9090/app.css", "http://127.0.0.1:9000/beacon.png"} {
		if got := local.DecideRequest("GET", u, Subresource); got != BlockHost {
			t.Errorf("%s was allowed: %v", u, got)
		}
	}
	listed := local
	listed.AllowedHosts = append(listed.AllowedHosts, "localhost:9090")
	if got := listed.DecideRequest("GET", "http://localhost:9090/app.css", Subresource); got != Allow {
		t.Errorf("a listed asset port was refused: %v", got)
	}
	if got := local.DecideRequest("GET", "http://localhost:9090/", Navigation); got != BlockHost {
		t.Errorf("navigation off the local target was allowed: %v", got)
	}
	if got := local.DecideRequest("POST", "http://localhost:9090/x", Subresource); got != BlockHost {
		t.Errorf("a write off the target was allowed: %v", got)
	}
}

func TestDecideIsTheNavigationRule(t *testing.T) {
	ro := target(true)
	ro.Lookup = fakeLookup(map[string]string{"cdn.other.test": "93.184.216.34"})
	if ro.Decide("GET", "https://cdn.other.test/a.js") != BlockHost {
		t.Error("Decide without a kind must keep the strict host rule")
	}
}

func TestTheLookupCacheKeepsAnswersButNotTimeouts(t *testing.T) {
	old := resolve
	t.Cleanup(func() { resolve = old })
	var calls atomic.Int32
	release := make(chan struct{})
	resolve = func(_ context.Context, host string) ([]net.IP, error) {
		calls.Add(1)
		switch host {
		case "slow.cache.test":
			return nil, context.DeadlineExceeded
		case "wait.cache.test":
			<-release
		case "gone.cache.test":
			return nil, errors.New("no such host")
		}
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	ctx := context.Background()
	for range 3 {
		if _, err := defaultLookup(ctx, "ok.cache.test"); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("an answer was looked up %d times, want 1", n)
	}
	calls.Store(0)
	for range 2 {
		_, _ = defaultLookup(ctx, "gone.cache.test")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("a failed lookup was repeated %d times within the minute, want 1", n)
	}
	calls.Store(0)
	for range 2 {
		_, _ = defaultLookup(ctx, "slow.cache.test")
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("a timed-out lookup was cached: %d calls, want 2", n)
	}
	// Requests for a name being looked up wait for that one lookup.
	calls.Store(0)
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = defaultLookup(ctx, "wait.cache.test") }()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("concurrent lookups of one name ran %d times, want 1", n)
	}
	// The cache starts again when it is full.
	lookupCache.Lock()
	for i := range maxCached {
		lookupCache.m[fmt.Sprint("filler-", i)] = cached{at: time.Now()}
	}
	lookupCache.Unlock()
	_, _ = defaultLookup(ctx, "new.cache.test")
	lookupCache.Lock()
	n := len(lookupCache.m)
	lookupCache.Unlock()
	if n > 2 {
		t.Errorf("the full cache was not started again: %d entries", n)
	}
}

func TestKindOfAndTargetOrigin(t *testing.T) {
	if KindOf(true) != Navigation || KindOf(false) != Subresource {
		t.Error("KindOf maps the wrong way")
	}
	ro := target(true)
	if !ro.TargetOrigin("https://www.example.test/a") || ro.TargetOrigin("https://cdn.other.test/a.js") || ro.TargetOrigin("::bad") {
		t.Error("TargetOrigin is wrong")
	}
}
