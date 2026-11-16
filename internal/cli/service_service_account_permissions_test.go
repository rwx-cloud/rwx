package cli_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestServiceAccountPermissionReplacement(t *testing.T) {
	for _, tc := range []struct {
		name, desired, current, unremovable, wantError string
		clear, yes, tty                                bool
		input                                          string
		status                                         int
	}{
		{name: "scoped and unscoped", desired: `[{"key":"run:read"},{"key":"run:read","resource_id":"allowed"}]`, yes: true},
		{name: "retained nongrantable", desired: `[{"key":"run:read","resource_id":"other"}]`, current: `[{"key":"run:read","resource_id":"other"}]`, yes: true},
		{name: "new nongrantable", desired: `[{"key":"run:read","resource_id":"other"}]`, yes: true, wantError: "cannot be granted"},
		{name: "cannot widen scope", desired: `[{"key":"vault:read"}]`, current: `[{"key":"vault:read","resource_id":"allowed"}]`, yes: true, wantError: "cannot be granted"},
		{name: "protected removal", clear: true, yes: true, current: `[{"key":"run:read","resource_id":"locked"}]`, unremovable: `["run:read(id: locked)"]`, wantError: "cannot be removed"},
		{name: "clear", clear: true, yes: true, current: `[{"key":"run:read"}]`},
		{name: "noninteractive replacement", desired: `[]`, wantError: "--yes"},
		{name: "noninteractive clear", clear: true, wantError: "--yes"},
		{name: "declined", clear: true, tty: true, input: "n\n", wantError: "aborted"},
		{name: "confirmed", clear: true, tty: true, input: "yes\n"},
		{name: "strict error", desired: `[]`, yes: true, status: 422, wantError: "strict validation failed"},
		{name: "authority changed", desired: `[]`, yes: true, status: 403, wantError: "strict validation failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.current == "" {
				tc.current = `[]`
			}
			if tc.unremovable == "" {
				tc.unremovable = `[]`
			}
			if tc.desired == "" {
				tc.desired = `[]`
			}
			var stdout, stderr bytes.Buffer
			puts := 0
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				status, body := 200, ""
				switch req.URL.Path {
				case "/mint/api/service_accounts":
					require.Equal(t, "Label", req.URL.Query().Get("display_name"))
					require.Empty(t, req.URL.Query().Get("name"))
					body = `{"service_accounts":[{"id":"account","name":"immutable"}],"pagination":{}}`
				case "/mint/api/permission_catalog":
					require.Equal(t, "account", req.URL.Query().Get("service_account_id"))
					require.Empty(t, req.URL.Query().Get("group_id"))
					body = `{"permissions":[{"key":"run:read","supports_resource_scope":true,"grantable":{"unscoped":true,"resource_ids":["allowed"]}},{"key":"vault:read","supports_resource_scope":true,"grantable":{"unscoped":false,"resource_ids":["allowed"]}}],"unremovable_permissions":` + tc.unremovable + `}`
				case "/mint/api/service_accounts/account/permissions":
					body = `{"permissions":` + tc.current + `}`
					if req.Method == "PUT" {
						puts++
						encoded, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						require.JSONEq(t, `{"permissions":`+tc.desired+`}`, string(encoded))
						body = string(encoded)
						if tc.status != 0 {
							status, body = tc.status, `{"errors":["strict validation failed"]}`
						}
					}
				default:
					t.Fatalf("unexpected request %s %s", req.Method, req.URL)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			svc := cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(tc.input), StderrIsTTY: tc.tty}}
			selector := cli.ServiceAccountSelector{DisplayName: "Label"}
			var result *cli.ServiceAccountPermissionsResult
			var err error
			if tc.clear {
				result, err = svc.ClearServiceAccountPermissions(cli.ClearServiceAccountPermissionsConfig{ServiceAccountSelector: selector, Yes: tc.yes, Json: true})
			} else {
				file := filepath.Join(t.TempDir(), "permissions.json")
				require.NoError(t, os.WriteFile(file, []byte(`{"permissions":`+tc.desired+`}`), 0600))
				result, err = svc.ReplaceServiceAccountPermissions(cli.ReplaceServiceAccountPermissionsConfig{ServiceAccountSelector: selector, File: file, Yes: tc.yes, Json: true})
			}
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, result)
				require.Empty(t, stdout.String())
				if tc.status == 403 {
					require.ErrorIs(t, err, errors.ErrUnauthenticated)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, "account", result.ServiceAccountID)
				require.Contains(t, stdout.String(), `"ServiceAccountID":"account"`)
				if tc.desired == `[]` {
					require.Contains(t, stdout.String(), `"Permissions":[]`)
				}
			}
			if tc.wantError == "" || tc.status != 0 {
				require.Equal(t, 1, puts)
			} else {
				require.Zero(t, puts)
			}
			if tc.tty {
				require.Contains(t, stderr.String(), "Inherited group permissions are unchanged")
			}
		})
	}
}

func TestShowServiceAccountDirectPermissions(t *testing.T) {
	for _, useJSON := range []bool{false, true} {
		var stdout bytes.Buffer
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			body := `{"service_accounts":[{"id":"account","name":"immutable"}],"pagination":{}}`
			if req.URL.Path == "/mint/api/service_accounts" {
				require.Equal(t, "immutable", req.URL.Query().Get("name"))
			} else {
				require.Equal(t, "/mint/api/service_accounts/account/permissions", req.URL.Path)
				body = `{"permissions":[{"key":"run:read"},{"key":"run:read","resource_id":"specific"}]}`
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		svc := cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout}}
		result, err := svc.ShowServiceAccountPermissions(cli.ShowServiceAccountPermissionsConfig{ServiceAccountSelector: cli.ServiceAccountSelector{ServiceAccount: "immutable"}, Json: useJSON})
		require.NoError(t, err)
		require.Len(t, result.Permissions, 2)
		if useJSON {
			require.JSONEq(t, `{"ServiceAccountID":"account","Permissions":[{"Key":"run:read","ResourceID":null},{"Key":"run:read","ResourceID":"specific"}]}`, stdout.String())
		} else {
			require.Contains(t, stdout.String(), "Direct permissions (inherited permissions unchanged)")
			require.Contains(t, stdout.String(), "(unscoped)")
			require.Contains(t, stdout.String(), "specific")
		}
	}
}
