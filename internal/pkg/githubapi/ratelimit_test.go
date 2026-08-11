package githubapi

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v50/github"
)

func TestRateLimitStateWaitFor(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	resetIn10m := now.Add(10 * time.Minute)

	tests := []struct {
		name  string
		state RateLimitState
		want  time.Duration
	}{
		{
			name:  "nothing observed yet",
			state: RateLimitState{},
			want:  0,
		},
		{
			name:  "requests still available",
			state: RateLimitState{Known: true, Remaining: 4000, Reset: resetIn10m},
			want:  0,
		},
		{
			name:  "a low but working limit is not waited for",
			state: RateLimitState{Known: true, Remaining: 3, Reset: resetIn10m},
			want:  0,
		},
		{
			name:  "exhausted waits for the reset",
			state: RateLimitState{Known: true, Remaining: 0, Reset: resetIn10m, Exhausted: true},
			want:  10*time.Minute + resetGrace,
		},
		{
			name:  "reset already passed",
			state: RateLimitState{Known: true, Remaining: 0, Reset: now.Add(-time.Minute), Exhausted: true},
			want:  0,
		},
		{
			name:  "exhausted without a known reset",
			state: RateLimitState{Known: true, Exhausted: true},
			want:  0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.state.WaitFor(now); got != test.want {
				t.Errorf("WaitFor() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestObserveResponse(t *testing.T) {
	reset := time.Date(2026, 8, 11, 12, 30, 0, 0, time.UTC)

	t.Run("response headers are recorded", func(t *testing.T) {
		ResetObservedRateLimit()
		ObserveResponse(&github.Response{Rate: github.Rate{
			Limit:     5000,
			Remaining: 4200,
			Reset:     github.Timestamp{Time: reset},
		}}, nil)
		state := CurrentRateLimit()
		if !state.Known || state.Remaining != 4200 || !state.Reset.Equal(reset) || state.Exhausted {
			t.Errorf("unexpected state: %+v", state)
		}
	})

	t.Run("rate limit error marks exhaustion", func(t *testing.T) {
		ResetObservedRateLimit()
		ObserveResponse(&github.Response{}, &github.RateLimitError{
			Rate:     github.Rate{Limit: 5000, Remaining: 0, Reset: github.Timestamp{Time: reset}},
			Response: &http.Response{},
		})
		state := CurrentRateLimit()
		if !state.Known || !state.Exhausted || state.Remaining != 0 || !state.Reset.Equal(reset) {
			t.Errorf("unexpected state: %+v", state)
		}
	})

	t.Run("secondary rate limit uses retry-after", func(t *testing.T) {
		ResetObservedRateLimit()
		retryAfter := 90 * time.Second
		before := time.Now()
		ObserveResponse(nil, &github.AbuseRateLimitError{RetryAfter: &retryAfter})
		state := CurrentRateLimit()
		if !state.Known || !state.Exhausted {
			t.Fatalf("unexpected state: %+v", state)
		}
		if state.Reset.Before(before.Add(retryAfter)) || state.Reset.After(time.Now().Add(retryAfter)) {
			t.Errorf("Reset = %v, want roughly now + %v", state.Reset, retryAfter)
		}
	})

	t.Run("a successful response clears exhaustion", func(t *testing.T) {
		ResetObservedRateLimit()
		ObserveResponse(&github.Response{}, &github.RateLimitError{Response: &http.Response{}})
		ObserveResponse(&github.Response{Rate: github.Rate{Limit: 5000, Remaining: 4999}}, nil)
		if state := CurrentRateLimit(); state.Exhausted {
			t.Errorf("Exhausted should have been cleared: %+v", state)
		}
	})

	t.Run("responses without rate headers keep the previous state", func(t *testing.T) {
		ResetObservedRateLimit()
		ObserveResponse(&github.Response{Rate: github.Rate{Limit: 5000, Remaining: 123}}, nil)
		ObserveResponse(nil, nil)
		ObserveResponse(&github.Response{}, errors.New("some other failure"))
		if state := CurrentRateLimit(); state.Remaining != 123 {
			t.Errorf("Remaining = %d, want 123", state.Remaining)
		}
	})
}
