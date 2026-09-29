package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestCronsRequests(t *testing.T) {
	for _, action := range []string{"show", "pause", "resume", "snooze"} {
		t.Run(action, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				path := "/mint/api/crons/id%2Fwith%20space"
				method := http.MethodPost
				status := http.StatusOK
				body := `{"cron":{"id":"id/with space","key":"nightly","repository":"org/repo","branch":"main","run_definition_path":".rwx/ci.yml","schedule":"0 0 * * *","time_zone":"America/New_York","status":"paused","next_invocation_at":null,"snoozed_until":null}}`
				switch action {
				case "show":
					method = http.MethodGet
				default:
					path += "/" + action
				}
				require.Equal(t, method, req.Method)
				require.Equal(t, path, req.URL.EscapedPath())
				if action == "snooze" {
					encoded, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.JSONEq(t, `{"until":"2027-01-02T09:30:59-05:00"}`, string(encoded))
					require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var result *api.CronResult
			var err error
			switch action {
			case "show":
				result, err = client.ShowCron("id/with space")
			case "pause":
				result, err = client.PauseCron("id/with space")
			case "resume":
				result, err = client.ResumeCron("id/with space")
			case "snooze":
				result, err = client.SnoozeCron("id/with space", "2027-01-02T09:30:59-05:00")
			}
			require.NoError(t, err)
			require.Equal(t, "id/with space", result.Cron.ID)
			require.Equal(t, ".rwx/ci.yml", result.Cron.RunDefinitionPath)
			require.Equal(t, "America/New_York", result.Cron.TimeZone)
			require.Nil(t, result.Cron.NextInvocationAt)
		})
	}
}

func TestListCrons(t *testing.T) {
	client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/mint/api/crons", req.URL.Path)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"crons":[{"id":"cron-1","next_invocation_at":"2027-01-02T10:00:00Z","snoozed_until":"2027-01-02T09:30:00Z"}]}`))}, nil
	})
	result, err := client.ListCrons()
	require.NoError(t, err)
	require.Len(t, result.Crons, 1)
	require.Equal(t, "2027-01-02T10:00:00Z", *result.Crons[0].NextInvocationAt)
	require.Equal(t, "2027-01-02T09:30:00Z", *result.Crons[0].SnoozedUntil)
}

func TestCronAPIErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"archived", 422, `{"errors":["Cron is archived."]}`, "Cron is archived."},
		{"not found", 404, `{}`, "404 Not Found"},
		{"permission denied", 403, `{"error_messages":[{"message":"You do not have permission to perform this action. It requires the \"cron:manage\" permission."}],"required_permission":"cron:manage"}`, `It requires the "cron:manage" permission.`},
		{"invalid snooze", 422, `{"errors":["Until must be in the future."]}`, "Until must be in the future."},
		{"invalid response", 200, `invalid`, "unable to parse API response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			result, err := client.SnoozeCron("cron-1", "2027-01-02T09:30:00Z")
			require.Nil(t, result)
			require.ErrorContains(t, err, tc.want)
		})
	}
}
