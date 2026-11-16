package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestResolveGroupAcrossPages(t *testing.T) {
	for _, tc := range []struct {
		name, selector, secondPage string
		status                     int
		wantID, wantError          string
	}{
		{"later name", "Release", `{"groups":[{"id":"target","name":"Release"}]}`, 200, "target", ""},
		{"later ID", "target", `{"groups":[{"id":"target","name":"Release"}]}`, 200, "target", ""},
		{"case sensitive", "release", `{"groups":[{"id":"target","name":"Release"}]}`, 200, "", "was not found"},
		{"duplicate name", "Build", `{"groups":[{"id":"other","name":"Build"}]}`, 200, "", "ambiguous"},
		{"ID name collision", "Build", `{"groups":[{"id":"Build","name":"Other"}]}`, 200, "", "ambiguous"},
		{"later failure after match", "Build", `{"error":"later page denied"}`, 403, "", "later page denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "100", req.URL.Query().Get("limit"))
				body, status := `{"groups":[{"id":"first","name":"Build"}],"pagination":{"next_cursor":"page+2"}}`, 200
				if req.URL.Query().Get("cursor") == "page+2" {
					body, status = tc.secondPage, tc.status
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			s := cli.Service{Config: cli.Config{APIClient: client}}
			group, err := s.ResolveGroup(tc.selector)
			if tc.wantError != "" {
				require.Nil(t, group)
				require.ErrorContains(t, err, tc.wantError)
				if tc.wantError == "was not found" {
					require.ErrorIs(t, err, errors.ErrNotFound)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantID, group.ID)
		})
	}
}

func TestDeleteGroupSafety(t *testing.T) {
	for _, tc := range []struct {
		name, input  string
		tty, yes     bool
		serverStatus int
		wantError    string
		deleted      bool
	}{
		{"noninteractive", "", false, false, 204, "use --yes", false},
		{"decline", "n\n", true, false, 204, "aborted", false},
		{"EOF", "", true, false, 204, "no input", false},
		{"confirm", "yes\n", true, false, 204, "", true},
		{"explicit yes", "", false, true, 204, "", true},
		{"protected", "", false, true, 409, "only source of admin permissions", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deleted := false
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.Method == "DELETE" {
					deleted = tc.serverStatus == 204
					return &http.Response{StatusCode: tc.serverStatus, Body: io.NopCloser(strings.NewReader(`{"errors":["only source of admin permissions"]}`))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"groups":[{"id":"id","name":"Release"}]}`))}, nil
			})
			var out, stderr bytes.Buffer
			s := cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: &stderr, StderrIsTTY: tc.tty, Stdin: strings.NewReader(tc.input)}}
			result, err := s.DeleteGroup(cli.DeleteGroupConfig{Group: "Release", Yes: tc.yes, Json: true})
			require.Equal(t, tc.deleted, deleted)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, result)
				require.Empty(t, out.String())
			} else {
				require.NoError(t, err)
				require.JSONEq(t, `{"ID":"id","Name":"Release","Description":null}`, out.String())
			}
		})
	}
}

func TestListGroupsJSONAndText(t *testing.T) {
	for _, jsonOutput := range []bool{true, false} {
		var out, stderr bytes.Buffer
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "7", req.URL.Query().Get("limit"))
			require.Equal(t, "start", req.URL.Query().Get("cursor"))
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"groups":null,"pagination":{"next_cursor":"next","limit":7}}`))}, nil
		})
		s := cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: &stderr}}
		result, err := s.ListGroups(cli.ListGroupsConfig{Limit: 7, Cursor: "start", Json: jsonOutput})
		require.NoError(t, err)
		require.NotNil(t, result.Groups)
		if jsonOutput {
			var value map[string]any
			require.NoError(t, json.Unmarshal(out.Bytes(), &value))
			require.JSONEq(t, `{"Groups":[],"Pagination":{"NextCursor":"next","Limit":7}}`, out.String())
			require.Empty(t, stderr.String())
		} else {
			require.Contains(t, out.String(), "No groups found.")
			require.Contains(t, stderr.String(), "--cursor next")
		}
	}
}
