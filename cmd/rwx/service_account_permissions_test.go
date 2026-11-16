package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestServiceAccountPermissionSelectors(t *testing.T) {
	permissions := findSubcommand(serviceAccountsCmd, "permissions")
	for _, name := range []string{"show", "replace", "clear"} {
		require.NotNil(t, findSubcommand(permissions, name))
	}
	for _, constructor := range []func() *cobra.Command{newServiceAccountPermissionsShowCommand, newServiceAccountPermissionsReplaceCommand, newServiceAccountPermissionsClearCommand} {
		cmd := constructor()
		require.Error(t, cmd.Args(cmd, nil))
		require.NoError(t, cmd.Args(cmd, []string{"immutable"}))
		require.Error(t, cmd.Args(cmd, []string{"one", "two"}))
		require.NoError(t, cmd.Flags().Set("display-name", "Label"))
		require.NoError(t, cmd.Args(cmd, nil))
		require.Error(t, cmd.Args(cmd, []string{"immutable"}))
	}
	require.ErrorContains(t, newServiceAccountPermissionsReplaceCommand().ValidateRequiredFlags(), "file")
	require.Contains(t, permissions.Long, "privileged")
}

func TestServiceAccountPermissionReplaceFlags(t *testing.T) {
	for _, output := range []string{"format", "json"} {
		t.Run(output, func(t *testing.T) {
			originalService, originalFormat := service, Format
			jsonFlag := rootCmd.PersistentFlags().Lookup("json")
			originalJSON := jsonFlag.Value.String()
			t.Cleanup(func() {
				service, Format = originalService, originalFormat
				require.NoError(t, jsonFlag.Value.Set(originalJSON))
			})
			Format = "json"
			require.NoError(t, jsonFlag.Value.Set("false"))
			if output == "json" {
				Format = "text"
				require.NoError(t, jsonFlag.Value.Set("true"))
			}
			var stdout bytes.Buffer
			puts := 0
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				body := `{"permissions":[]}`
				switch req.URL.Path {
				case "/mint/api/service_accounts":
					require.Equal(t, "Label", req.URL.Query().Get("display_name"))
					body = `{"service_accounts":[{"id":"account","name":"immutable"}],"pagination":{}}`
				case "/mint/api/permission_catalog":
					body = `{"permissions":[{"key":"run:read","grantable":{"unscoped":true}}]}`
				case "/mint/api/service_accounts/account/permissions":
					if req.Method == "PUT" {
						puts++
						data, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						require.JSONEq(t, `{"permissions":[{"key":"run:read"}]}`, string(data))
						body = string(data)
					}
				default:
					t.Fatalf("unexpected request %s", req.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout, Stdin: strings.NewReader(`{"permissions":[{"key":"run:read"}]}`)}}
			cmd := newServiceAccountPermissionsReplaceCommand()
			require.NoError(t, cmd.ParseFlags([]string{"--display-name", "Label", "--file", "-", "--yes"}))
			require.NoError(t, cmd.Args(cmd, nil))
			require.NoError(t, cmd.ValidateRequiredFlags())
			require.NoError(t, cmd.RunE(cmd, nil))
			require.Equal(t, 1, puts)
			require.JSONEq(t, `{"ServiceAccountID":"account","Permissions":[{"Key":"run:read","ResourceID":null}]}`, stdout.String())
		})
	}
}
