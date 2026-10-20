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

func TestServiceAccountCommands(t *testing.T) {
	require.Same(t, serviceAccountsCmd, findSubcommand(rootCmd, "service-accounts"))
	for _, name := range []string{"list", "show", "create", "update", "deactivate", "permissions"} {
		require.NotNil(t, findSubcommand(serviceAccountsCmd, name))
	}
	for _, constructor := range []func() *cobra.Command{newServiceAccountsShowCommand, newServiceAccountsUpdateCommand, newServiceAccountsDeactivateCommand} {
		cmd := constructor()
		require.Error(t, cmd.Args(cmd, nil))
		require.NoError(t, cmd.Args(cmd, []string{"immutable-name"}))
		require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
		require.NoError(t, cmd.Flags().Set("display-name", "Label"))
		require.NoError(t, cmd.Args(cmd, nil))
		require.Error(t, cmd.Args(cmd, []string{"immutable-name"}))
	}
	require.ErrorContains(t, newServiceAccountsCreateCommand().ValidateRequiredFlags(), "display-name")
}

func TestServiceAccountFlagUpdates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
		body  string
	}{
		{"rename", []string{"--new-display-name", "New Label"}, `{"display_name":"New Label"}`},
		{"clear", []string{"--description", ""}, `{"description":""}`},
		{"description", []string{"--description", "release automation"}, `{"description":"release automation"}`},
		{"both", []string{"--new-display-name", "New Label", "--description", ""}, `{"display_name":"New Label","description":""}`},
	} {
		for _, format := range []string{"json", "flag"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				originalService, originalFormat := service, Format
				t.Cleanup(func() { service, Format = originalService, originalFormat })
				Format = "json"
				if format == "flag" {
					Format = "text"
					require.NoError(t, rootCmd.PersistentFlags().Lookup("json").Value.Set("true"))
				}
				var stdout bytes.Buffer
				patched := false
				client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					if req.Method == "PATCH" {
						patched = true
						require.Equal(t, "/mint/api/service_accounts/account", req.URL.Path)
						body, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						require.JSONEq(t, tc.body, string(body))
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"service_account":{"id":"account","identity_id":"identity","name":"immutable","display_name":"New Label","description":""}}`))}, nil
					}
					require.Equal(t, "Old Label", req.URL.Query().Get("display_name"))
					require.Empty(t, req.URL.Query().Get("name"))
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"service_accounts":[{"id":"account","name":"immutable","display_name":"Old Label","description":"keep me"}],"pagination":{}}`))}, nil
				})
				service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout}}
				cmd := newServiceAccountsUpdateCommand()
				require.NoError(t, cmd.ParseFlags(append([]string{"--display-name", "Old Label"}, tc.flags...)))
				require.NoError(t, cmd.Args(cmd, nil))
				require.NoError(t, cmd.RunE(cmd, nil))
				require.True(t, patched)
				require.JSONEq(t, `{"ID":"account","IdentityID":"identity","Name":"immutable","DisplayName":"New Label","Description":""}`, stdout.String())
			})
		}
	}
}

func TestServiceAccountCreateFlags(t *testing.T) {
	originalService, originalFormat := service, Format
	t.Cleanup(func() { service, Format = originalService, originalFormat })
	Format = "json"
	var stdout bytes.Buffer
	client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "POST", req.Method)
		require.Equal(t, "/mint/api/service_accounts", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"display_name":"Deploy Bot","description":"release automation"}`, string(body))
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"service_account":{"id":"account","identity_id":"identity","name":"deploy-bot","display_name":"Deploy Bot","description":"release automation"}}`))}, nil
	})
	service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout}}
	cmd := newServiceAccountsCreateCommand()
	require.NoError(t, cmd.ParseFlags([]string{"--display-name", "Deploy Bot", "--description", "release automation"}))
	require.NoError(t, cmd.RunE(cmd, nil))
	require.JSONEq(t, `{"ID":"account","IdentityID":"identity","Name":"deploy-bot","DisplayName":"Deploy Bot","Description":"release automation"}`, stdout.String())
}
