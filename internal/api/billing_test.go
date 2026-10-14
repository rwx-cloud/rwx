package api_test

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestBillingUsageRequests(t *testing.T) {
	fixture, err := os.ReadFile("testdata/billing_usage.json")
	require.NoError(t, err)
	for _, month := range []string{"", "2026-09"} {
		t.Run(month, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, "/api/billing/usage", req.URL.Path)
				require.Equal(t, "application/json", req.Header.Get("Accept"))
				if month == "" {
					require.Empty(t, req.URL.RawQuery)
				} else {
					require.Equal(t, "month=2026-09", req.URL.RawQuery)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(fixture)))}, nil
			})
			result, err := client.BillingUsage(month)
			require.NoError(t, err)
			require.Equal(t, "acme", result.OrganizationSlug)
			require.Equal(t, "2026-09-30", result.Period.EndsOn)
			require.Equal(t, "2026-09-01", *result.Period.FinalizedThrough)
			require.Equal(t, "83.450000000000000001", result.UsageCostInDollars)
			require.Equal(t, "17.123456789012345678", result.AvailableCreditInDollars)
			require.Equal(t, "172.50000000000000005", result.Summary[0].Quantity)
			require.Equal(t, "0.02", *result.Summary[0].UnitPriceInDollars)
			require.Equal(t, api.BillingUnit{Code: "gib_day", Label: "GiB-day"}, result.Summary[0].Unit)
			require.Equal(t, []string{"Standard"}, result.Summary[0].Qualifiers)
			require.True(t, result.Days[0].Lines[1].Included)
			require.Nil(t, result.Days[0].Lines[1].UnitPriceInDollars)
			require.Equal(t, "0.0", *result.Days[0].Lines[1].CostInDollars)
			require.True(t, result.Days[1].Processing)
			require.Nil(t, result.Days[1].CostInDollars)
			require.Empty(t, result.Days[1].Lines)
			require.False(t, result.Days[2].Processing)
			require.Equal(t, "1.450000000000000001", *result.Days[2].CostInDollars)
		})
	}
}

func TestBillingUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		status   int
		body     string
		want     string
		sentinel error
	}{
		{400, `{"error_messages":[{"message":"Month must not be in the future."}]}`, "Month must not be in the future.", nil},
		{401, `{"error":"Unauthorized"}`, "requires authentication", errors.ErrUnauthenticated},
		{403, `{"errors":["Access denied."]}`, "Access denied.", errors.ErrUnauthenticated},
		{404, `{}`, "404 Not Found", api.ErrNotFound},
		{500, `{"errors":["Billing unavailable."]}`, "Billing unavailable.", errors.ErrInternalServerError},
		{200, `invalid`, "unable to parse API response", nil},
	} {
		t.Run(tc.want, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			result, err := client.BillingUsage("9999-12")
			require.Nil(t, result)
			require.ErrorContains(t, err, tc.want)
			if tc.sentinel != nil {
				require.ErrorIs(t, err, tc.sentinel)
			}
		})
	}
	t.Run("transport", func(t *testing.T) {
		client := api.NewClientWithRoundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
		_, err := client.BillingUsage("")
		require.ErrorContains(t, err, "HTTP request failed: offline")
	})
}
