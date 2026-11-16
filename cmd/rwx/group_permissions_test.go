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

func TestGroupPermissionsCommands(t *testing.T) {
	require.NotNil(t, findSubcommand(groupsCmd, "permissions"))
	for _, tc := range []struct {
		args      []string
		wantError string
	}{
		{[]string{"show"}, "accepts 1 arg"},
		{[]string{"clear", "Release", "extra"}, "accepts 1 arg"},
		{[]string{"replace", "Release", "--yes"}, "required flag"},
		{[]string{"replace", "Release", "--file", "-", "--yes"}, ""},
		{[]string{"clear", "Release", "--yes"}, ""},
		{[]string{"show", "Release"}, ""},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			originalService, originalFormat := service, Format
			t.Cleanup(func() { service, Format = originalService, originalFormat })
			Format = "json"
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				if tc.wantError != "" {
					t.Fatal("invalid command must not reach API")
				}
				body := `{"groups":[{"id":"id","name":"Release"}]}`
				if strings.HasSuffix(req.URL.Path, "/permissions") || req.URL.Path == "/mint/api/permission_catalog" {
					body = `{"permissions":[]}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var out bytes.Buffer
			service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: io.Discard, Stdin: strings.NewReader(`{"permissions":[]}`)}}
			cmd := newGroupPermissionsCommand()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Empty(t, out.String())
			} else {
				require.NoError(t, err)
				require.JSONEq(t, `{"GroupID":"id","Permissions":[]}`, out.String())
			}
		})
	}
}
