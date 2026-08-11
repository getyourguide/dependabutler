package githubapi

import (
	"errors"
	"sync"
	"time"

	"github.com/google/go-github/v50/github"
)

// resetGrace is added to every wait, so a request is not retried a fraction of a
// second before the rate limit window has actually rolled over.
const resetGrace = 5 * time.Second

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

var (
	rateStateMutex sync.Mutex
	rateState      RateLimitState
)

// ObserveResponse records the rate limit reported by a GitHub API call. Both
// arguments may be nil. A rate limit error takes precedence over the response,
// because go-github returns an empty response when it blocks a request locally
// on the strength of a previously seen limit.
func ObserveResponse(resp *github.Response, err error) {
	rateStateMutex.Lock()
	defer rateStateMutex.Unlock()

	var rateErr *github.RateLimitError
	if errors.As(err, &rateErr) {
		rateState = RateLimitState{
			Known:     true,
			Remaining: rateErr.Rate.Remaining,
			Reset:     rateErr.Rate.Reset.Time,
			Exhausted: true,
		}
		return
	}

	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		reset := time.Now()
		if abuseErr.RetryAfter != nil {
			reset = reset.Add(*abuseErr.RetryAfter)
		}
		rateState = RateLimitState{Known: true, Reset: reset, Exhausted: true}
		return
	}

	if resp == nil || resp.Rate.Limit == 0 {
		// no rate limit headers to learn from, keep what we had
		return
	}
	rateState = RateLimitState{
		Known:     true,
		Remaining: resp.Rate.Remaining,
		Reset:     resp.Rate.Reset.Time,
	}
}

// CurrentRateLimit returns the most recently observed rate limit state.
func CurrentRateLimit() RateLimitState {
	rateStateMutex.Lock()
	defer rateStateMutex.Unlock()
	return rateState
}

// ResetObservedRateLimit discards the observed rate limit state.
func ResetObservedRateLimit() {
	rateStateMutex.Lock()
	defer rateStateMutex.Unlock()
	rateState = RateLimitState{}
}
