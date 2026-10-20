package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/dummy-run/dummy-run/internal/robots"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

// robotsGuard returns a check to run before a sitemap is requested. With respect off it returns nil, so
// nothing is checked. Otherwise it reads robots.txt once for the origin of each address and refuses an
// address the rules disallow for Dummy Run.
func robotsGuard(client *http.Client, identify, respect bool) func(context.Context, string) error {
	if !respect {
		return nil
	}
	var mu sync.Mutex
	cache := map[string]*robots.Result{}
	return func(ctx context.Context, loc string) error {
		origin, err := robots.Origin(loc)
		if err != nil {
			return err
		}
		mu.Lock()
		res := cache[origin]
		mu.Unlock()
		if res == nil {
			if res, err = robots.Fetch(ctx, client, loc, telemetry.RobotsToken, telemetry.RequestUserAgent(identify)); err != nil {
				return fmt.Errorf("robots.txt: %w", err)
			}
			mu.Lock()
			cache[origin] = res
			mu.Unlock()
		}
		u, err := url.Parse(loc)
		if err != nil {
			return err
		}
		if !res.Rules.Allowed(u.RequestURI()) {
			return fmt.Errorf("robots.txt disallows DummyRun from the sitemap %s (%s)", loc, res.Status)
		}
		return nil
	}
}
