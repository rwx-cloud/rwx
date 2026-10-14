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

func TestBillingUsageCommand(t *testing.T) {
	require.Same(t, billingCmd, findSubcommand(rootCmd, "billing"))
	require.Same(t, billingUsageCmd, findSubcommand(billingCmd, "usage"))
	for _, tc := range []struct {
		args    []string
		month   string
		invalid bool
	}{
		{args: nil},
		{args: []string{"--month", "2026-09"}, month: "2026-09"},
		{args: []string{"--month", "9999-12"}, month: "9999-12"},
		{args: []string{"--month", "2026-9"}, invalid: true},
		{args: []string{"--month", "2026-13"}, invalid: true},
		{args: []string{"--month", ""}, invalid: true},
		{args: []string{"2026-09"}, invalid: true},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			originalService, originalFormat := service, Format
			t.Cleanup(func() { service, Format = originalService, originalFormat })
			Format = "json"
			var output bytes.Buffer
			called := false
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				called = true
				require.Equal(t, tc.month, req.URL.Query().Get("month"))
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"organization_slug":"acme","summary":[],"days":[]}`))}, nil
			})
			service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &output}}
			cmd := newBillingUsageCommand()
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if tc.invalid {
				require.Error(t, err)
				require.False(t, called)
				require.Empty(t, output.String())
			} else {
				require.NoError(t, err)
				require.True(t, called)
				require.Contains(t, output.String(), `"OrganizationSlug":"acme"`)
				require.Contains(t, output.String(), `"Summary":[],"Days":[]`)
			}
		})
	}
}
