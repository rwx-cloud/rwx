package api

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/retry"
	"github.com/stretchr/testify/require"
)

// stubRetrySleep replaces the package-level sleep for the duration of a test,
// recording the delays it was asked to wait instead of actually sleeping.
func stubRetrySleep(t *testing.T) *[]time.Duration {
	t.Helper()
	original := listRunsRetrySleep
	var slept []time.Duration
	listRunsRetrySleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { listRunsRetrySleep = original })
	return &slept
}

func okRunsResponse() *http.Response {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"runs":[],"pagination":{"next_cursor":null,"limit":50}}`)),
	}
}

func rateLimitedResponse(retryAfter string) *http.Response {
	header := http.Header{}
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{
		Status:     "429 Too Many Requests",
		StatusCode: 429,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

func TestListRuns_Retry(t *testing.T) {
	t.Run("retries a 429 honoring Retry-After, then succeeds", func(t *testing.T) {
		slept := stubRetrySleep(t)

		attempts := 0
		c := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return rateLimitedResponse("60"), nil
			}
			return okRunsResponse(), nil
		})

		var progress bytes.Buffer
		result, err := c.ListRuns(ListRunsConfig{RetryProgress: &progress})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 2, attempts)
		require.Equal(t, []time.Duration{60 * time.Second}, *slept)
		require.Contains(t, progress.String(), "rate limited by RWX")
		require.Contains(t, progress.String(), "retrying in 60s")
		require.Contains(t, progress.String(), "attempt 2/5")
	})

	t.Run("retries transient 5xx with exponential backoff", func(t *testing.T) {
		slept := stubRetrySleep(t)

		attempts := 0
		c := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts < 3 {
				return &http.Response{
					Status:     "503 Service Unavailable",
					StatusCode: 503,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}
			return okRunsResponse(), nil
		})

		result, err := c.ListRuns(ListRunsConfig{})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 3, attempts)
		// No Retry-After, so backoff doubles: 1s then 2s.
		require.Equal(t, []time.Duration{1 * time.Second, 2 * time.Second}, *slept)
	})

	t.Run("retries an empty / non-JSON 200 body", func(t *testing.T) {
		stubRetrySleep(t)

		attempts := 0
		c := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			return okRunsResponse(), nil
		})

		result, err := c.ListRuns(ListRunsConfig{})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 2, attempts)
	})

	t.Run("gives up after the max attempts and surfaces the last error", func(t *testing.T) {
		slept := stubRetrySleep(t)

		attempts := 0
		c := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			attempts++
			return rateLimitedResponse("5"), nil
		})

		maxAttempts := retry.NewBackoff().MaxFailures
		_, err := c.ListRuns(ListRunsConfig{})
		require.Error(t, err)
		var rateErr *RateLimitedError
		require.True(t, errors.As(err, &rateErr))
		require.Equal(t, maxAttempts, attempts)
		// One fewer sleep than attempts: the final failure is not followed by a sleep.
		require.Len(t, *slept, maxAttempts-1)
	})

	t.Run("does not retry a non-retryable 4xx", func(t *testing.T) {
		slept := stubRetrySleep(t)

		attempts := 0
		c := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			attempts++
			raw := `{"errors":[{"key":"result_status","provided_value":"nope","suggested_value":"failed","valid_values":["failed"]}]}`
			return &http.Response{Status: "400 Bad Request", StatusCode: 400, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})

		_, err := c.ListRuns(ListRunsConfig{})
		require.Error(t, err)
		var filterErr *InvalidRunFilterError
		require.True(t, errors.As(err, &filterErr))
		require.Equal(t, 1, attempts)
		require.Empty(t, *slept)
	})

	t.Run("stays silent when no RetryProgress writer is set", func(t *testing.T) {
		stubRetrySleep(t)

		attempts := 0
		c := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return rateLimitedResponse("1"), nil
			}
			return okRunsResponse(), nil
		})

		// A nil RetryProgress must not panic; the retry still happens.
		_, err := c.ListRuns(ListRunsConfig{})
		require.NoError(t, err)
		require.Equal(t, 2, attempts)
	})
}
