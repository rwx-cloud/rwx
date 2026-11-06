package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestGroupsCommands(t *testing.T) {
	require.Same(t, groupsCmd, findSubcommand(rootCmd, "groups"))
	require.Len(t, groupsCmd.Commands(), 5)
	for _, name := range []string{"show", "update", "delete"} {
		cmd := findSubcommand(groupsCmd, name)
		require.Error(t, cmd.Args(cmd, nil))
		require.NoError(t, cmd.Args(cmd, []string{"Release"}))
		require.Error(t, cmd.Args(cmd, []string{"Release", "Other"}))
	}
	out, err := executeRoot(t, "groups")
	require.NoError(t, err)
	require.Contains(t, out, "Available Commands")
	require.Contains(t, out, "create")
}

func TestGroupsMetadataFlags(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		command                              func() *cobra.Command
		args                                 []string
		wantName, wantDescription, wantError string
	}{
		{"rename", newGroupsUpdateCommand, []string{"Release", "--name", "Shipping"}, "Shipping", "existing description", ""},
		{"clear description", newGroupsUpdateCommand, []string{"Release", "--description", ""}, "Release", "", ""},
		{"set description", newGroupsUpdateCommand, []string{"Release", "--description", "new description"}, "Release", "new description", ""},
		{"create", newGroupsCreateCommand, []string{"--name", "New", "--description", "new description"}, "New", "new description", ""},
		{"no updates", newGroupsUpdateCommand, []string{"Release"}, "", "", "at least one field"},
		{"empty name", newGroupsUpdateCommand, []string{"Release", "--name", ""}, "", "", "name must not be empty"},
		{"missing name", newGroupsCreateCommand, nil, "", "", "required flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalService, originalFormat := service, Format
			t.Cleanup(func() { service, Format = originalService, originalFormat })
			Format = "json"
			name, description := "Release", "existing description"
			identityState := map[string]any{"permissions": []any{"org:admin"}, "memberships": []any{"member"}, "relationships": []any{"child"}}
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				if tc.wantError != "" {
					t.Fatal("invalid input must not reach API")
				}
				if req.Method == "PATCH" || req.Method == "POST" {
					var payload map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
					for key, value := range payload {
						switch key {
						case "name":
							name = value.(string)
						case "description":
							description = value.(string)
						default:
							identityState[key] = value
						}
					}
					body, err := json.Marshal(map[string]any{"group": map[string]any{"id": "stable-id", "name": name, "description": description}})
					require.NoError(t, err)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"groups":[{"id":"stable-id","name":"Release","description":"existing description"}]}`))}, nil
			})
			var out bytes.Buffer
			service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: io.Discard}}
			cmd := tc.command()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantName, name)
			require.Equal(t, tc.wantDescription, description)
			require.Equal(t, map[string]any{"permissions": []any{"org:admin"}, "memberships": []any{"member"}, "relationships": []any{"child"}}, identityState)
			var result map[string]any
			require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			require.Equal(t, "stable-id", result["ID"])
			require.Equal(t, tc.wantName, result["Name"])
			require.Equal(t, tc.wantDescription, result["Description"])
		})
	}
}

func TestGroupsListAndShowFlags(t *testing.T) {
	originalService, originalFormat := service, Format
	t.Cleanup(func() { service, Format = originalService, originalFormat })
	Format = "text"
	client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := `{"group":{"id":"stable-id","name":"Release","description":"team"}}`
		if req.URL.Path == "/mint/api/groups" {
			if req.URL.Query().Get("limit") != "100" {
				require.Equal(t, "7", req.URL.Query().Get("limit"))
				require.Equal(t, "opaque+cursor", req.URL.Query().Get("cursor"))
			}
			body = `{"groups":[{"id":"stable-id","name":"Release","description":"team"}],"pagination":{"limit":7}}`
		} else {
			require.Equal(t, "/mint/api/groups/stable-id", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	var out bytes.Buffer
	service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: io.Discard}}
	list := newGroupsListCommand()
	list.SetArgs([]string{"--limit", "7", "--cursor", "opaque+cursor"})
	require.NoError(t, list.Execute())
	require.Contains(t, out.String(), "ID")
	require.Contains(t, out.String(), "NAME")
	require.Contains(t, out.String(), "DESCRIPTION")
	require.Contains(t, out.String(), "Release")
	out.Reset()
	show := newGroupsShowCommand()
	show.SetArgs([]string{"Release"})
	require.NoError(t, show.Execute())
	require.Contains(t, out.String(), "stable-id")
	require.Contains(t, out.String(), "team")
}
