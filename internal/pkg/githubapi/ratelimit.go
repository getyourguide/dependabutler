package githubapi

import (
	"errors"
	"time"

	"github.com/google/go-github/v50/github"
)

// resetGrace is added to every wait, so a request is not retried a fraction of a
// second before the rate limit window has actually rolled over.
const resetGrace = 5 * time.Second

// secondaryLimitFallback is how long to wait after hitting a secondary rate
// limit that came without a Retry-After header. GitHub does not guarantee the
// header, and retrying straight into a secondary limit tends to extend it.
const secondaryLimitFallback = time.Minute

// RateLimitState is a snapshot of the GitHub API rate limit, taken from the
// X-RateLimit-* headers of the most recent API response.
//
// The GET /rate_limit endpoint is deliberately not used. It has been observed
// reporting an untouched budget (used=0, remaining=5000) with a reset sliding
// along with wall-clock time, while the counter enforced on the same token's
// other requests had already been spent. Response headers are what GitHub's own
// documentation recommends relying on.
type RateLimitState struct {
	// Known is false until an API response has been observed.
	Known bool
	// Remaining is the number of requests left in the current window.
	Remaining int
	// Reset is the time the current window rolls over.
	Reset time.Time
	// Exhausted is true if the last API call was rejected because a primary or
	// secondary rate limit had been reached.
	Exhausted bool
}

// WaitFor returns how long to pause before the next API request, or zero if no
// pause is needed: the limit has not been reached, or its reset has already
// passed. No safety buffer is needed, because every API call reports the limit
// it saw, so exhaustion is noticed as soon as it happens.
func (s RateLimitState) WaitFor(now time.Time) time.Duration {
	if !s.Exhausted {
		return 0
	}
	if !s.Reset.After(now) {
		return 0
	}
	return s.Reset.Sub(now) + resetGrace
}

// observe records the rate limit reported by a GitHub API call. Both arguments
// may be nil. A rate limit error takes precedence over the response, because
// go-github returns an empty response when it blocks a request locally on the
// strength of a previously seen limit.
func (c *Client) observe(resp *github.Response, err error) {
	c.rateMutex.Lock()
	defer c.rateMutex.Unlock()

	var rateErr *github.RateLimitError
	if errors.As(err, &rateErr) {
		c.rate = RateLimitState{
			Known:     true,
			Remaining: rateErr.Rate.Remaining,
			Reset:     rateErr.Rate.Reset.Time,
			Exhausted: true,
		}
		return
	}

	// A secondary rate limit (go-github still calls it "abuse", the name GitHub
	// used before renaming it) is a separate mechanism from the hourly quota: it
	// is triggered by the shape of the traffic rather than its volume - too many
	// concurrent requests, or too many content-creating ones such as commits and
	// pull requests in a burst. The hourly budget can be nearly untouched when it
	// fires, so X-RateLimit-Remaining says nothing useful here and Retry-After is
	// the only signal for how long to hold off.
	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		retryAfter := secondaryLimitFallback
		if abuseErr.RetryAfter != nil && *abuseErr.RetryAfter > 0 {
			retryAfter = *abuseErr.RetryAfter
		}
		c.rate = RateLimitState{
			Known:     true,
			Reset:     time.Now().Add(retryAfter),
			Exhausted: true,
		}
		return
	}

	if resp == nil || resp.Rate.Limit == 0 {
		// no rate limit headers to learn from, keep what we had
		return
	}
	c.rate = RateLimitState{
		Known:     true,
		Remaining: resp.Rate.Remaining,
		Reset:     resp.Rate.Reset.Time,
	}
}

// RateLimit returns the rate limit state observed on this client's most recent
// API response.
func (c *Client) RateLimit() RateLimitState {
	c.rateMutex.Lock()
	defer c.rateMutex.Unlock()
	return c.rate
}
