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

func TestGroupRelationshipAPI(t *testing.T) {
	pair := api.GroupRelationship{ParentGroupID: "parent +/?", ChildGroupID: "child &/?"}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, method, req.Method)
				require.Equal(t, "/mint/api/group_relationships", req.URL.Path)
				if method == "GET" {
					require.Equal(t, pair.ParentGroupID, req.URL.Query().Get("parent_group_id"))
					require.Equal(t, pair.ChildGroupID, req.URL.Query().Get("child_group_id"))
				} else {
					var body map[string]string
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					require.Equal(t, map[string]string{"parent_group_id": "parent +/?", "child_group_id": "child &/?"}, body)
					require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				}
				status := 200
				if method == "POST" {
					status = 201
				}
				if method == "DELETE" {
					return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"group_relationship":{"parent_group_id":"parent +/?","child_group_id":"child &/?"}}`))}, nil
			})
			if method == "DELETE" {
				require.NoError(t, client.RemoveGroupRelationship(pair))
				return
			}
			var result *api.GroupRelationshipResult
			var err error
			if method == "GET" {
				result, err = client.ShowGroupRelationship(pair)
			} else {
				result, err = client.AddGroupRelationship(pair)
			}
			require.NoError(t, err)
			require.Equal(t, pair, result.GroupRelationship)
		})
	}
}

func TestGroupRelationshipAPIErrors(t *testing.T) {
	for _, tc := range []struct {
		status        int
		body, message string
		sentinel      error
	}{
		{404, "", "not found", errors.ErrNotFound},
		{403, `{"error_messages":[{"message":"group:manage required on parent"}]}`, "group:manage required on parent", errors.ErrUnauthenticated},
		{409, `{"errors":["Relationship would create a cycle"]}`, "Relationship would create a cycle", nil},
		{409, `{"errors":["Only source of admin permissions"]}`, "Only source of admin permissions", nil},
	} {
		for _, action := range []string{"show", "add", "remove"} {
			t.Run(action+tc.message, func(t *testing.T) {
				client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				})
				pair := api.GroupRelationship{ParentGroupID: "p", ChildGroupID: "c"}
				var err error
				switch action {
				case "show":
					_, err = client.ShowGroupRelationship(pair)
				case "add":
					_, err = client.AddGroupRelationship(pair)
				case "remove":
					err = client.RemoveGroupRelationship(pair)
				}
				require.ErrorContains(t, err, tc.message)
				if tc.sentinel != nil {
					require.ErrorIs(t, err, tc.sentinel)
				}
			})
		}
	}
}
