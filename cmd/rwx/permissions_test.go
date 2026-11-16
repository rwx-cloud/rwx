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

func TestPermissionsListCommand(t *testing.T) {
	require.Same(t, permissionsCmd, findSubcommand(rootCmd, "permissions"))
	require.Same(t, permissionsListCmd, findSubcommand(permissionsCmd, "list"))
	require.NoError(t, permissionsListCmd.Args(permissionsListCmd, nil))
	require.Error(t, permissionsListCmd.Args(permissionsListCmd, []string{"extra"}))
	group := permissionsListCmd.Flags().Lookup("group-id")
	account := permissionsListCmd.Flags().Lookup("service-account-id")
	oldGroup, oldAccount := group.Changed, account.Changed
	t.Cleanup(func() { group.Changed, account.Changed = oldGroup, oldAccount })
	group.Changed, account.Changed = false, false
	require.NoError(t, permissionsListCmd.ValidateFlagGroups())
	group.Changed = true
	require.NoError(t, permissionsListCmd.ValidateFlagGroups())
	account.Changed = true
	require.Error(t, permissionsListCmd.ValidateFlagGroups())
	group.Changed = false
	require.NoError(t, permissionsListCmd.ValidateFlagGroups())
}

func TestPermissionsListJSONFlags(t *testing.T) {
	for _, tc := range []struct {
		formatFlag string
		value      string
		targetFlag string
		queryKey   string
	}{
		{"json", "true", "group-id", "group_id"},
		{"format", "json", "service-account-id", "service_account_id"},
	} {
		t.Run(tc.formatFlag, func(t *testing.T) {
			originalService, originalFormat := service, Format
			t.Cleanup(func() { service, Format = originalService, originalFormat })
			flag := rootCmd.PersistentFlags().Lookup(tc.formatFlag)
			originalChanged := flag.Changed
			t.Cleanup(func() { flag.Changed = originalChanged })
			require.NoError(t, rootCmd.PersistentFlags().Set(tc.formatFlag, tc.value))
			target := permissionsListCmd.Flags().Lookup(tc.targetFlag)
			originalTarget, originalTargetChanged := target.Value.String(), target.Changed
			t.Cleanup(func() {
				require.NoError(t, target.Value.Set(originalTarget))
				target.Changed = originalTargetChanged
			})
			require.NoError(t, permissionsListCmd.Flags().Set(tc.targetFlag, "target-one"))
			var output bytes.Buffer
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, tc.queryKey+"=target-one", req.URL.RawQuery)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"permissions":[],"unremovable_permissions":[]}`))}, nil
			})
			service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &output}}
			require.NoError(t, permissionsListCmd.RunE(permissionsListCmd, nil))
			require.JSONEq(t, `{"Permissions":[],"UnremovablePermissions":[]}`, output.String())
		})
	}
}
