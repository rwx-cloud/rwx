package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryAfterDelay(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		delay time.Duration
		valid bool
	}{
		{"0", 0, true},
		{" 17 ", 17 * time.Second, true},
		{now.Add(23 * time.Second).Format(http.TimeFormat), 23 * time.Second, true},
		{now.Add(-time.Second).Format(http.TimeFormat), 0, true},
		{"", 0, false},
		{"-1", 0, false},
		{"+1", 0, false},
		{"1.5", 0, false},
		{"tomorrow", 0, false},
		{"9223372037", 0, false},
		{"999999999999999999999", 0, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			delay, valid := retryAfterDelay(tc.value, now)
			require.Equal(t, tc.valid, valid)
			require.Equal(t, tc.delay, delay)
		})
	}
}

type retryResponseBody struct {
	io.Reader
	closed bool
}

func (b *retryResponseBody) Close() error {
	b.closed = true
	return nil
}

func TestClientRetry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		header    string
		streaming bool
		succeedAt int
		attempts  int
	}{
		{name: "replays POST body", status: 503, header: "0", succeedAt: 3, attempts: 3},
		{name: "stops after five attempts", status: 503, header: "0", attempts: 5},
		{name: "missing header", status: 503, attempts: 1},
		{name: "invalid header", status: 503, header: "invalid", attempts: 1},
		{name: "other status", status: 500, header: "0", attempts: 1},
		{name: "rate limit unchanged", status: 429, header: "0", attempts: 1},
		{name: "non replayable body", status: 503, header: "0", streaming: true, attempts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies []*retryResponseBody
			attempts := 0
			client := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				if attempts > 0 {
					require.True(t, bodies[attempts-1].closed)
				}
				attempts++
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, `{"action":"create","count":7}`, string(body))
				require.NoError(t, req.Body.Close())
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				responseBody := &retryResponseBody{Reader: strings.NewReader("response")}
				bodies = append(bodies, responseBody)
				status := tc.status
				if attempts == tc.succeedAt {
					status = http.StatusOK
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {tc.header}}, Body: responseBody}, nil
			})
			req, err := http.NewRequest(http.MethodPost, "/api/example", strings.NewReader(`{"action":"create","count":7}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			if tc.streaming {
				req.GetBody = nil
			}
			resp, err := client.RoundTrip(req)
			require.NoError(t, err)
			require.Equal(t, tc.attempts, attempts)
			require.False(t, bodies[len(bodies)-1].closed)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, "response", string(body))
			if tc.succeedAt > 0 {
				require.Equal(t, http.StatusOK, resp.StatusCode)
			} else {
				require.Equal(t, tc.status, resp.StatusCode)
			}
		})
	}
}

func TestClientRetryWait(t *testing.T) {
	t.Run("honors delay", func(t *testing.T) {
		attempts := 0
		start := time.Now()
		client := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			attempts++
			status := http.StatusServiceUnavailable
			if attempts == 2 {
				require.GreaterOrEqual(t, time.Since(start), time.Second)
				status = http.StatusOK
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"1"}}, Body: http.NoBody}, nil
		})
		req, err := http.NewRequest(http.MethodGet, "/api/example", nil)
		require.NoError(t, err)
		resp, err := client.RoundTrip(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, 2, attempts)
	})
	t.Run("cancels wait", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		body := &retryResponseBody{Reader: strings.NewReader("")}
		attempts := 0
		client := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": {"60"}}, Body: body}, nil
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/example", nil)
		require.NoError(t, err)
		resp, err := client.RoundTrip(req)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, resp)
		require.True(t, body.closed)
		require.Equal(t, 1, attempts)
	})
}

func TestListRuns503RetryAfter(t *testing.T) {
	slept := stubRetrySleep(t)
	attempts := 0
	client := NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": {"7"}}, Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`))}, nil
	})
	_, err := client.ListRuns(ListRunsConfig{})
	require.EqualError(t, err, "unavailable")
	require.Equal(t, 5, attempts)
	require.Equal(t, []time.Duration{7 * time.Second, 7 * time.Second, 7 * time.Second, 7 * time.Second}, *slept)
}
