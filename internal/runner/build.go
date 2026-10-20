package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxBuildBody bounds how much of the home page is hashed.
const maxBuildBody = 4 << 20

// BuildHash identifies the deployed build of a URL target that has no commit: the first 12 hex
// characters of the SHA-256 of the home page body followed by its Last-Modified and ETag headers.
// Two runs against the same deployment give the same hash unless the home page changes between
// requests, for example with a nonce or a timestamp; then each run looks like its own build and
// "dummyrun coverage" refuses to pool them. Give the target a commit to avoid that.
func BuildHash(ctx context.Context, client *http.Client, baseURL string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("home page status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBuildBody))
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(body)
	h.Write([]byte(resp.Header.Get("Last-Modified")))
	h.Write([]byte(resp.Header.Get("ETag")))
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}
