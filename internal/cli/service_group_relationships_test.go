package cli_test

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

func TestRemoveGroupRelationshipSafety(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		tty, yes    bool
		status      int
		wantError   string
	}{
		{"noninteractive", "", false, false, 204, "use --yes"},
		{"decline", "n\n", true, false, 204, "aborted"},
		{"EOF", "", true, false, 204, "no input"},
		{"confirm", "yes\n", true, false, 204, ""},
		{"explicit yes", "", false, true, 204, ""},
		{"protected", "", false, true, 409, "only source of admin permissions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			removed := false
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/mint/api/groups" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"groups":[{"id":"p","name":"Parent"},{"id":"c","name":"Child"}]}`))}, nil
				}
				require.Equal(t, "/mint/api/group_relationships", req.URL.Path)
				require.Equal(t, "DELETE", req.Method)
				removed = tc.status == 204
				body := ""
				if tc.status != 204 {
					body = `{"errors":["only source of admin permissions"]}`
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var out, stderr bytes.Buffer
			s := cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: &stderr, Stdin: strings.NewReader(tc.input), StderrIsTTY: tc.tty}}
			result, err := s.RemoveGroupRelationship(cli.GroupRelationshipConfig{Parent: "Parent", Child: "Child", Yes: tc.yes, Json: true})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, result)
				require.Empty(t, out.String())
				require.False(t, removed)
			} else {
				require.NoError(t, err)
				require.True(t, removed)
				require.JSONEq(t, `{"ParentGroupID":"p","ChildGroupID":"c"}`, out.String())
			}
			if tc.tty {
				require.Contains(t, stderr.String(), "from parent group p to child group c")
			}
		})
	}
}

func TestGroupRelationshipResolutionErrors(t *testing.T) {
	for _, run := range []func(cli.Service, cli.GroupRelationshipConfig) (*cli.GroupRelationshipResult, error){
		cli.Service.ShowGroupRelationship, cli.Service.AddGroupRelationship, cli.Service.RemoveGroupRelationship,
	} {
		for _, tc := range []struct{ parent, child, groups, message string }{
			{"", "Child", "", "provide both"},
			{"Parent", "", "", "provide both"},
			{"Missing", "Child", `{"groups":[]}`, "unable to resolve parent group"},
			{"Parent", "Missing", `{"groups":[{"id":"p","name":"Parent"}]}`, "unable to resolve child group"},
			{"Parent", "Child", `{"groups":[{"id":"p","name":"Parent"},{"id":"c1","name":"Child"},{"id":"c2","name":"Child"}]}`, "ambiguous"},
		} {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "/mint/api/groups", req.URL.Path)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.groups))}, nil
			})
			var out bytes.Buffer
			s := cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: io.Discard}}
			result, err := run(s, cli.GroupRelationshipConfig{Parent: tc.parent, Child: tc.child, Yes: true})
			require.ErrorContains(t, err, tc.message)
			require.Nil(t, result)
			require.Empty(t, out.String())
		}
	}
}
