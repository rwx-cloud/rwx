package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestServiceAccountRequests(t *testing.T) {
	empty, label := "", "New label"
	for _, tc := range []struct {
		name, method, endpoint, body string
		operation                    func(api.Client) error
	}{
		{"create", "POST", "/mint/api/service_accounts", `{"display_name":"Deploy Bot","description":""}`, func(c api.Client) error {
			_, err := c.CreateServiceAccount(api.CreateServiceAccountConfig{DisplayName: "Deploy Bot", Description: &empty})
			return err
		}},
		{"rename", "PATCH", "/mint/api/service_accounts/account%2Fid", `{"display_name":"New label"}`, func(c api.Client) error {
			_, err := c.UpdateServiceAccount(api.UpdateServiceAccountConfig{ID: "account/id", DisplayName: &label})
			return err
		}},
		{"clear description", "PATCH", "/mint/api/service_accounts/account%2Fid", `{"description":""}`, func(c api.Client) error {
			_, err := c.UpdateServiceAccount(api.UpdateServiceAccountConfig{ID: "account/id", Description: &empty})
			return err
		}},
		{"show", "GET", "/mint/api/service_accounts/account%2Fid", "", func(c api.Client) error {
			result, err := c.ShowServiceAccount("account/id")
			if err == nil {
				require.Equal(t, "immutable", result.ServiceAccount.Name)
				require.Equal(t, "Mutable", result.ServiceAccount.DisplayName)
				require.Equal(t, "identity", result.ServiceAccount.IdentityID)
				require.Nil(t, result.ServiceAccount.Description)
			}
			return err
		}},
		{"deactivate", "DELETE", "/mint/api/service_accounts/account%2Fid", "", func(c api.Client) error { return c.DeactivateServiceAccount("account/id") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, tc.method, req.Method)
				require.Equal(t, tc.endpoint, req.URL.String())
				require.Equal(t, "application/json", req.Header.Get("Accept"))
				if tc.body != "" {
					require.Equal(t, "application/json", req.Header.Get("Content-Type"))
					var body map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					encoded, err := json.Marshal(body)
					require.NoError(t, err)
					require.JSONEq(t, tc.body, string(encoded))
				}
				status := 200
				body := `{"service_account":{"id":"account/id","identity_id":"identity","name":"immutable","display_name":"Mutable","description":null}}`
				if tc.method == "POST" {
					status = 201
				}
				if tc.method == "DELETE" {
					status, body = 204, ""
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			require.NoError(t, tc.operation(client))
		})
	}
}

func TestServiceAccountsListWirePagination(t *testing.T) {
	client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "GET", req.Method)
		require.Equal(t, "/mint/api/service_accounts", req.URL.Path)
		require.Equal(t, "immutable", req.URL.Query().Get("name"))
		require.Equal(t, "Label & other", req.URL.Query().Get("display_name"))
		require.Equal(t, "17", req.URL.Query().Get("limit"))
		require.Equal(t, "opaque+/=?&", req.URL.Query().Get("cursor"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"service_accounts":[],"pagination":{"next_cursor":"next+/=","limit":17}}`))}, nil
	})
	result, err := client.ListServiceAccounts(api.ListServiceAccountsConfig{Name: "immutable", DisplayName: "Label & other", Limit: 17, Cursor: "opaque+/=?&"})
	require.NoError(t, err)
	require.NotNil(t, result.ServiceAccounts)
	require.Empty(t, result.ServiceAccounts)
	require.Equal(t, "next+/=", result.Pagination.NextCursor)
	require.Equal(t, 17, result.Pagination.Limit)
}

func TestServiceAccountErrors(t *testing.T) {
	for _, tc := range []struct {
		status        int
		body, message string
		sentinel      error
	}{
		{400, `{"error":"Invalid cursor"}`, "Invalid cursor", nil},
		{401, `{"error":"Unauthorized"}`, "rwx login", errors.ErrUnauthenticated},
		{403, `{"error_messages":[{"message":"Requires service_account:manage"}]}`, "Requires service_account:manage", errors.ErrUnauthenticated},
		{404, "", "404 Not Found", api.ErrNotFound},
		{409, `{"errors":["Name has already been taken"]}`, "Name has already been taken", nil},
		{422, `{"errors":["Could not deactivate the service account"]}`, "Could not deactivate the service account", nil},
		{500, "", "500 Internal Server Error", errors.ErrInternalServerError},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Status: fmt.Sprintf("%d %s", tc.status, http.StatusText(tc.status)), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			err := client.DeactivateServiceAccount("account")
			require.ErrorContains(t, err, tc.message)
			if tc.sentinel != nil {
				require.ErrorIs(t, err, tc.sentinel)
			}
		})
	}
}
