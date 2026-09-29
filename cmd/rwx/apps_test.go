package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestAppsCommands(t *testing.T) {
	require.Same(t, appsCmd, findSubcommand(rootCmd, "apps"))
	require.Equal(t, "api", appsCmd.GroupID)
	require.Len(t, appsCmd.Commands(), 2)
	for _, action := range []string{"disable", "enable"} {
		t.Run(action, func(t *testing.T) {
			cmd := findSubcommand(appsCmd, action)
			require.NotNil(t, cmd)
			require.Equal(t, action+" ENDPOINT_OR_URL", cmd.Use)
			require.Error(t, cmd.Args(cmd, nil))
			require.NoError(t, cmd.Args(cmd, []string{"pr-123"}))
			require.Error(t, cmd.Args(cmd, []string{"pr-123", "pr-456"}))

			originalService, originalFormat := service, Format
			t.Cleanup(func() { service, Format = originalService, originalFormat })
			Format = "json"
			var stdout bytes.Buffer
			called := false
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				called = true
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "/mint/api/app_endpoints/"+action, req.URL.String())
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"endpoint":"pr-123"}`, string(body))
				return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout}}
			require.NoError(t, cmd.RunE(cmd, []string{"pr-123"}))
			require.True(t, called)
			if action == "disable" {
				require.JSONEq(t, `{"Endpoint":"pr-123","Disabled":true}`, stdout.String())
			} else {
				require.JSONEq(t, `{"Endpoint":"pr-123","Disabled":false}`, stdout.String())
			}
		})
	}
}
