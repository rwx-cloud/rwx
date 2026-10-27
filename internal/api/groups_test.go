package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestGroupAPIRequests(t *testing.T) {
	empty, renamed := "", "Shipping"
	for _, tc := range []struct {
		name, method, path string
		body               map[string]any
		call               func(api.Client) error
	}{
		{"show", "GET", "/mint/api/groups/group%2Fid", nil, func(c api.Client) error { _, err := c.ShowGroup("group/id"); return err }},
		{"create", "POST", "/mint/api/groups", map[string]any{"name": "Release"}, func(c api.Client) error { _, err := c.CreateGroup(api.CreateGroupConfig{Name: "Release"}); return err }},
		{"rename preserves description", "PATCH", "/mint/api/groups/group%2Fid", map[string]any{"name": "Shipping"}, func(c api.Client) error {
			_, err := c.UpdateGroup("group/id", api.UpdateGroupConfig{Name: &renamed})
			return err
		}},
		{"clear preserves name", "PATCH", "/mint/api/groups/group%2Fid", map[string]any{"description": ""}, func(c api.Client) error {
			_, err := c.UpdateGroup("group/id", api.UpdateGroupConfig{Description: &empty})
			return err
		}},
		{"delete", "DELETE", "/mint/api/groups/group%2Fid", nil, func(c api.Client) error { return c.DeleteGroup("group/id") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, tc.method, req.Method)
				require.Equal(t, tc.path, req.URL.EscapedPath())
				require.Equal(t, "application/json", req.Header.Get("Accept"))
				if tc.body != nil {
					var body map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					require.Equal(t, tc.body, body)
					require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				}
				if tc.method == "DELETE" {
					return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"group":{"id":"group/id","name":"Release","description":null}}`))}, nil
			})
			require.NoError(t, tc.call(c))
		})
	}
}

func TestGroupAPIListPagination(t *testing.T) {
	c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/mint/api/groups", req.URL.Path)
		require.Equal(t, "7", req.URL.Query().Get("limit"))
		require.Equal(t, "opaque +/?", req.URL.Query().Get("cursor"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"groups":[{"id":"id","name":"Release","description":"team"}],"pagination":{"next_cursor":"next","limit":7}}`))}, nil
	})
	result, err := c.ListGroups(api.ListGroupsConfig{Limit: 7, Cursor: "opaque +/?"})
	require.NoError(t, err)
	require.Equal(t, "team", *result.Groups[0].Description)
	require.Equal(t, "next", *result.Pagination.NextCursor)
	require.Equal(t, 7, result.Pagination.Limit)
}

func TestGroupAPIErrors(t *testing.T) {
	for _, tc := range []struct {
		status        int
		body, message string
		sentinel      error
	}{
		{404, "", "not found", errors.ErrNotFound},
		{403, `{"error_messages":[{"message":"group:manage required"}]}`, "group:manage required", errors.ErrUnauthenticated},
		{409, `{"errors":["System groups cannot be edited or deleted."]}`, "System groups cannot be edited or deleted.", nil},
		{409, `{"errors":["This group is the only source of admin permissions"]}`, "only source of admin permissions", nil},
		{422, `{"errors":["Name has already been taken"]}`, "Name has already been taken", nil},
		{400, `{"error":"Invalid cursor"}`, "Invalid cursor", nil},
	} {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})
		err := c.DeleteGroup("id")
		require.ErrorContains(t, err, tc.message)
		if tc.sentinel != nil {
			require.ErrorIs(t, err, tc.sentinel)
		}
	}
}
