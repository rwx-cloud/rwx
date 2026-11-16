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

func TestGroupRelationshipCommands(t *testing.T) {
	for _, action := range []string{"show", "add", "remove"} {
		for _, output := range []string{"text", "json", "format json"} {
			t.Run(action+" "+output, func(t *testing.T) {
				originalService, originalFormat := service, Format
				t.Cleanup(func() { service, Format = originalService, originalFormat })
				Format = "text"
				var out bytes.Buffer
				client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					if req.URL.Path == "/mint/api/groups" {
						if req.URL.Query().Get("cursor") == "next" {
							return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"groups":[{"id":"child-id","name":"Child"}]}`))}, nil
						}
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"groups":[{"id":"parent-id","name":"Parent"}],"pagination":{"next_cursor":"next"}}`))}, nil
					}
					require.Equal(t, "/mint/api/group_relationships", req.URL.Path)
					status := 200
					switch action {
					case "show":
						require.Equal(t, "GET", req.Method)
						require.Equal(t, "parent-id", req.URL.Query().Get("parent_group_id"))
						require.Equal(t, "child-id", req.URL.Query().Get("child_group_id"))
					case "add", "remove":
						var pair map[string]string
						require.NoError(t, json.NewDecoder(req.Body).Decode(&pair))
						require.Equal(t, map[string]string{"parent_group_id": "parent-id", "child_group_id": "child-id"}, pair)
						if action == "remove" {
							require.Equal(t, "DELETE", req.Method)
							return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
						}
						require.Equal(t, "POST", req.Method)
						status = 201
					}
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"group_relationship":{"parent_group_id":"parent-id","child_group_id":"child-id"}}`))}, nil
				})
				service = cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: io.Discard}}
				cmd := &cobra.Command{Use: "rwx"}
				cmd.PersistentFlags().StringVar(&Format, "format", "text", "")
				cmd.PersistentFlags().Var(&jsonFlagValue{format: &Format}, "json", "")
				cmd.PersistentFlags().Lookup("json").NoOptDefVal = "true"
				cmd.AddCommand(newGroupRelationshipsCommand())
				args := []string{"relationships", action, "--parent", "Parent", "--child", "child-id"}
				if action == "remove" {
					args = append(args, "--yes")
				}
				if output == "json" {
					args = append(args, "--json")
				}
				if output == "format json" {
					args = append(args, "--format", "json")
				}
				cmd.SetArgs(args)
				require.NoError(t, cmd.Execute())
				if output != "text" {
					var result map[string]string
					require.NoError(t, json.Unmarshal(out.Bytes(), &result))
					require.Equal(t, map[string]string{"ParentGroupID": "parent-id", "ChildGroupID": "child-id"}, result)
				} else if action == "remove" {
					require.Contains(t, out.String(), "from parent group parent-id to child group child-id")
					require.Contains(t, out.String(), "Neither group was deleted")
				} else {
					require.Contains(t, out.String(), "Child group child-id")
					require.Contains(t, out.String(), "inherits permissions from parent group parent-id")
				}
			})
		}
	}
}

func TestGroupRelationshipRequiredSelectors(t *testing.T) {
	for _, action := range []string{"show", "add", "remove"} {
		for _, selectors := range [][]string{nil, {"--parent", "Parent"}, {"--child", "Child"}} {
			cmd := newGroupRelationshipsCommand()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(append([]string{action}, selectors...))
			require.ErrorContains(t, cmd.Execute(), "required flag")
		}
		cmd := findSubcommand(newGroupRelationshipsCommand(), action)
		require.Error(t, cmd.Args(cmd, []string{"unexpected"}))
	}
}
