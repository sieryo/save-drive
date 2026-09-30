package gdrive

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRetryBackoff = time.Second
	retryAttempts       = 5
	maxRetryWait        = 30 * time.Second
)

// withRetry wraps hc so that requests failing with a transient error (network
// error, 429, 5xx, or Drive's 403 rate-limit reasons) are retried with
// exponential backoff. Requests whose body cannot be replayed, such as
// streaming media uploads, are never retried here; the Drive library retries
// resumable upload chunks itself.
func withRetry(hc *http.Client, backoff time.Duration) *http.Client {
	base := hc.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	out := *hc
	out.Transport = &retryTransport{base: base, backoff: backoff}
	return &out
}

type retryTransport struct {
	base    http.RoundTripper
	backoff time.Duration
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	replayable := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	for attempt := 1; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if attempt >= retryAttempts || !replayable || ctx.Err() != nil || !shouldRetry(resp, err) {
			return resp, err
		}

		wait := t.backoff << (attempt - 1)
		if resp != nil {
			if ra, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && ra > 0 {
				wait = time.Duration(ra) * time.Second
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		wait = min(wait, maxRetryWait)

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		next := req.Clone(ctx)
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			next.Body = body
		}
		req = next
	}
}

// rateLimitReasons are the Drive error reasons reported with HTTP 403 that are
// safe to retry.
var rateLimitReasons = []string{"rateLimitExceeded", "userRateLimitExceeded"}

func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		// Transport errors (reset connection, DNS hiccup) are transient; context
		// cancellation is filtered out by the caller before this is consulted.
		return true
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusForbidden:
		// Peek at the body for the rate-limit reason, then restore it so the
		// caller can still read the error details.
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(b))
		for _, reason := range rateLimitReasons {
			if strings.Contains(string(b), reason) {
				return true
			}
		}
	}
	return false
}
