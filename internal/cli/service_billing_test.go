package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestService_BillingUsage(t *testing.T) {
	fixture, err := os.ReadFile("../api/testdata/billing_usage.json")
	require.NoError(t, err)
	var response api.BillingUsageResult
	require.NoError(t, json.Unmarshal(fixture, &response))
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprint(jsonOutput), func(t *testing.T) {
			s := setupTest(t)
			s.mockAPI.MockBillingUsage = func(month string) (*api.BillingUsageResult, error) {
				require.Equal(t, "2026-09", month)
				return &response, nil
			}
			result, err := s.service.BillingUsage(cli.BillingUsageConfig{Month: "2026-09", Json: jsonOutput})
			require.NoError(t, err)
			require.Equal(t, "83.450000000000000001", result.UsageCostInDollars)
			require.Equal(t, "2026-09-01", *result.Period.FinalizedThrough)
			require.Equal(t, "172.50000000000000005", result.Summary[0].Quantity)
			require.Len(t, result.Days, 3)
			output := s.mockStdout.String()
			if jsonOutput {
				var decoded map[string]any
				require.NoError(t, json.Unmarshal([]byte(output), &decoded))
				require.Len(t, decoded, 7)
				require.Equal(t, "acme", decoded["OrganizationSlug"])
				require.Equal(t, "USD", decoded["Currency"])
				require.Equal(t, "17.123456789012345678", decoded["AvailableCreditInDollars"])
				require.JSONEq(t, `{"Month":"2026-09","StartsOn":"2026-09-01","EndsOn":"2026-09-30","FinalizedThrough":"2026-09-01"}`, mustBillingJSON(t, decoded["Period"]))
				summary := decoded["Summary"].([]any)
				require.Len(t, summary, 2)
				require.JSONEq(t, `{"SKU":"artifact_storage.gib_day","Label":"Artifact storage","Qualifiers":["Standard"],"Unit":{"Code":"gib_day","Label":"GiB-day"},"Quantity":"172.50000000000000005","UnitPriceInDollars":"0.02","Included":false,"CostInDollars":"3.450000000000000001"}`, mustBillingJSON(t, summary[0]))
				days := decoded["Days"].([]any)
				require.JSONEq(t, `{"Date":"2026-09-02","CostInDollars":null,"Lines":[]}`, mustBillingJSON(t, days[1]))
				included := days[0].(map[string]any)["Lines"].([]any)[1]
				require.JSONEq(t, `{"SKU":"egress.aggregate.gib","Label":"External data transfer","Qualifiers":[],"Unit":{"Code":"gib","Label":"GiB"},"Quantity":"5.25","UnitPriceInDollars":null,"Included":true,"CostInDollars":"0.0"}`, mustBillingJSON(t, included))
				require.Equal(t, "1.450000000000000001", days[2].(map[string]any)["CostInDollars"])
			} else {
				for _, want := range []string{"Organization: acme", "Period: 2026-09-01 to 2026-09-30", "Finalized through: 2026-09-01", "Usage cost: 83.450000000000000001", "Available credit: 17.123456789012345678", "artifact_storage.gib_day", "Standard", "GiB-day", "172.50000000000000005", "0.02", "abq.parallel_job"} {
					require.Contains(t, output, want)
				}
				require.Contains(t, strings.Join(strings.Fields(output), " "), "egress.aggregate.gib External data transfer 5.25 GiB gib - true 0.0")
				require.Contains(t, strings.Join(strings.Fields(output), " "), "2026-09-02 Cost: Pending")
				require.Contains(t, strings.Join(strings.Fields(output), " "), "2026-09-03 Cost: 1.450000000000000001")
				require.NotContains(t, output, "Finalized\t")
			}
		})
	}
}

func mustBillingJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func TestService_BillingUsageEmptyMonth(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		s := setupTest(t)
		response := &api.BillingUsageResult{OrganizationSlug: "empty", Currency: "USD", Period: api.BillingPeriod{Month: "2026-02", StartsOn: "2026-02-01", EndsOn: "2026-02-28"}, UsageCostInDollars: "0.0", AvailableCreditInDollars: "0.0", Summary: []api.BillingLine{}}
		zero := "0.0"
		for day := 1; day <= 28; day++ {
			response.Days = append(response.Days, api.BillingDay{Date: fmt.Sprintf("2026-02-%02d", day), CostInDollars: &zero, Lines: []api.BillingLine{}})
		}
		s.mockAPI.MockBillingUsage = func(month string) (*api.BillingUsageResult, error) {
			require.Empty(t, month)
			return response, nil
		}
		result, err := s.service.BillingUsage(cli.BillingUsageConfig{Json: jsonOutput})
		require.NoError(t, err)
		require.Nil(t, result.Period.FinalizedThrough)
		require.Empty(t, result.Summary)
		require.Len(t, result.Days, 28)
		if jsonOutput {
			require.Contains(t, s.mockStdout.String(), `"FinalizedThrough":null`)
			require.Contains(t, s.mockStdout.String(), `"Summary":[]`)
			require.Contains(t, s.mockStdout.String(), `{"Date":"2026-02-28","CostInDollars":"0.0","Lines":[]}`)
		} else {
			require.Contains(t, s.mockStdout.String(), "Finalized through: None")
			require.Contains(t, s.mockStdout.String(), "No summarized usage.")
			require.Contains(t, strings.Join(strings.Fields(s.mockStdout.String()), " "), "2026-02-28 Cost: 0.0")
			require.NotContains(t, s.mockStdout.String(), "Pending")
		}
	}
}

func TestService_BillingUsageValidationAndErrors(t *testing.T) {
	for _, month := range []string{"2026-9", "2026-00", "2026-13", "2026-09-01", "0000-01", " 2026-09", "tomorrow"} {
		s := setupTest(t)
		_, err := s.service.BillingUsage(cli.BillingUsageConfig{Month: month})
		require.ErrorContains(t, err, "month must be in YYYY-MM format")
		require.Empty(t, s.mockStdout.String())
	}
	s := setupTest(t)
	s.mockAPI.MockBillingUsage = func(month string) (*api.BillingUsageResult, error) {
		require.Equal(t, "9999-12", month)
		return nil, errors.New("Month must not be in the future.")
	}
	_, err := s.service.BillingUsage(cli.BillingUsageConfig{Month: "9999-12"})
	require.ErrorContains(t, err, "unable to retrieve billing usage: Month must not be in the future.")
	require.Empty(t, s.mockStdout.String())
}

func TestService_BillingUsageOutputError(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		s := setupTest(t)
		s.mockAPI.MockBillingUsage = func(string) (*api.BillingUsageResult, error) { return &api.BillingUsageResult{}, nil }
		s.service.Stdout = billingFailWriter{}
		_, err := s.service.BillingUsage(cli.BillingUsageConfig{Json: jsonOutput})
		require.ErrorIs(t, err, io.ErrClosedPipe)
	}
}

type billingFailWriter struct{}

func (billingFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
