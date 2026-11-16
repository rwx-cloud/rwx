package cli_test

import (
	"bytes"
	"encoding/json"
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

func TestReplaceGroupPermissionsValidation(t *testing.T) {
	const catalog = `{"permissions":[
		{"key":"run:read","grantable":{"unscoped":true}},
		{"key":"org:admin","grantable":{"unscoped":false}},
		{"key":"vault:manage","supports_resource_scope":true,"grantable":{"unscoped":false,"resource_ids":["allowed"]}},
		{"key":"vault:read","supports_resource_scope":true,"grantable":{"unscoped":true,"resource_ids":[]}}
	]}`
	for _, tc := range []struct {
		name, current, desired, unremovable, wantError string
	}{
		{"retain nongrantable and omit removable", `[{"key":"org:admin"},{"key":"run:read"}]`, `[{"key":"org:admin"}]`, `[]`, ""},
		{"new nongrantable", `[{"key":"run:read"}]`, `[{"key":"org:admin"}]`, `[]`, "cannot be granted"},
		{"retain inert scope", `[{"key":"vault:manage","resource_id":"deleted"}]`, `[{"key":"vault:manage","resource_id":"deleted"}]`, `[]`, ""},
		{"new inert scope", `[]`, `[{"key":"vault:manage","resource_id":"deleted"}]`, `[]`, "cannot be granted"},
		{"grant allowed scope", `[]`, `[{"key":"vault:manage","resource_id":"allowed"}]`, `[]`, ""},
		{"scoped grantability is not unscoped", `[]`, `[{"key":"vault:manage"}]`, `[]`, "cannot be granted"},
		{"unscoped grantability is not scoped", `[]`, `[{"key":"vault:read","resource_id":"allowed"}]`, `[]`, "cannot be granted"},
		{"grant unscoped", `[]`, `[{"key":"vault:read"}]`, `[]`, ""},
		{"cannot broaden retained scope", `[{"key":"vault:manage","resource_id":"allowed"}]`, `[{"key":"vault:manage"}]`, `[]`, "cannot be granted"},
		{"retain scoped unremovable", `[{"key":"vault:manage","resource_id":"deleted"},{"key":"run:read"}]`, `[{"key":"vault:manage","resource_id":"deleted"}]`, `["vault:manage(id: deleted)"]`, ""},
		{"omit scoped unremovable", `[{"key":"vault:manage","resource_id":"deleted"}]`, `[{"key":"run:read"}]`, `["vault:manage(id: deleted)"]`, "cannot be removed"},
		{"omit unscoped unremovable", `[{"key":"org:admin"}]`, `[]`, `["org:admin"]`, "cannot be removed"},
		{"unknown key even retained", `[{"key":"unknown"}]`, `[{"key":"unknown"}]`, `[]`, "unknown permission key"},
		{"unsupported scope even retained", `[{"key":"run:read","resource_id":"allowed"}]`, `[{"key":"run:read","resource_id":"allowed"}]`, `[]`, "does not support"},
		{"empty scope", `[]`, `[{"key":"vault:manage","resource_id":""}]`, `[]`, "does not support"},
		{"duplicate", `[]`, `[{"key":"run:read"},{"key":"run:read"}]`, `[]`, "duplicate permission"},
		{"explicit clear", `[{"key":"run:read"}]`, `[]`, `[]`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := tc.current
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				body := `{"groups":[{"id":"group/id","name":"Release"}]}`
				switch req.URL.Path {
				case "/mint/api/permission_catalog":
					require.Equal(t, "group/id", req.URL.Query().Get("group_id"))
					require.Empty(t, req.URL.Query().Get("service_account_id"))
					body = strings.TrimSuffix(catalog, "}") + `,"unremovable_permissions":` + tc.unremovable + `}`
				case "/mint/api/groups/group/id/permissions":
					if req.Method == http.MethodPut {
						data, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						require.JSONEq(t, `{"permissions":`+tc.desired+`}`, string(data))
						state = tc.desired
					}
					body = `{"permissions":` + state + `}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var out bytes.Buffer
			s := cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: io.Discard, Stdin: strings.NewReader(`{"permissions":` + tc.desired + `}`)}}
			result, err := s.ReplaceGroupPermissions(cli.ReplaceGroupPermissionsConfig{Group: "Release", File: "-", Yes: true, Json: true})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, result)
				require.Equal(t, tc.current, state)
				require.Empty(t, out.String())
				return
			}
			require.NoError(t, err)
			require.JSONEq(t, tc.desired, state)
			require.Equal(t, "group/id", result.GroupID)
			var decoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
			require.Contains(t, decoded, "Permissions")
			require.JSONEq(t, `"group/id"`, string(decoded["GroupID"]))
		})
	}
}

func TestGroupPermissionReplacementFileSafety(t *testing.T) {
	for _, input := range []string{
		``, `null`, `{}`, `{"permissions":null}`, `{"permissions":[null]}`,
		`{"permisions":[]}`, `{"permissions":[],"extra":true}`,
		`{"permissions":[{"key":"run:read","resource":"oops"}]}`,
		`{"permissions":[]} {"permissions":[]}`, `{"permissions":[]} null`,
		`{"permissions":[]} garbage`, `{"permissions":{}}`,
	} {
		t.Run(input, func(t *testing.T) {
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				t.Fatal("invalid replacement must not reach the API")
				return nil, nil
			})
			s := cli.Service{Config: cli.Config{APIClient: client, Stdin: strings.NewReader(input)}}
			_, err := s.ReplaceGroupPermissions(cli.ReplaceGroupPermissionsConfig{Group: "Release", File: "-", Yes: true})
			require.ErrorIs(t, err, errors.ErrBadRequest)
		})
	}
}

func TestGroupPermissionConfirmationAndFiles(t *testing.T) {
	for _, tc := range []struct {
		name, input, wantError string
		tty, yes, clear, file  bool
		status                 int
	}{
		{"replace noninteractive", "", "use --yes", false, false, false, false, 200},
		{"clear noninteractive", "", "use --yes", false, false, true, false, 200},
		{"decline", "n\n", "aborted", true, false, true, false, 200},
		{"EOF", "", "no input", true, false, true, false, 200},
		{"confirm clear", "yes\n", "", true, false, true, false, 200},
		{"confirm file", "y\n", "", true, false, false, true, 200},
		{"yes stdin", "", "", false, true, false, false, 200},
		{"server rejects atomically", "", "cannot remove protected permission", false, true, true, false, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := `[{"key":"run:read"}]`
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				body, status := `{"groups":[{"id":"id","name":"Release"}]}`, 200
				switch req.URL.Path {
				case "/mint/api/groups/id/permissions":
					if req.Method == http.MethodPut {
						data, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						require.JSONEq(t, `{"permissions":[]}`, string(data))
						if tc.status != 200 {
							return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(`{"errors":["cannot remove protected permission"]}`))}, nil
						}
						state = `[]`
					}
					body = `{"permissions":` + state + `}`
				case "/mint/api/permission_catalog":
					body = `{"permissions":[{"key":"run:read","grantable":{"unscoped":true}}]}`
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var out, stderr bytes.Buffer
			input := tc.input
			file := "-"
			if tc.file {
				file = filepath.Join(t.TempDir(), "permissions.json")
				require.NoError(t, os.WriteFile(file, []byte(`{"permissions":[]}`), 0600))
			} else if !tc.clear {
				input = `{"permissions":[]}`
			}
			s := cli.Service{Config: cli.Config{APIClient: client, Stdout: &out, Stderr: &stderr, Stdin: strings.NewReader(input), StderrIsTTY: tc.tty}}
			var err error
			if tc.clear {
				_, err = s.ClearGroupPermissions(cli.ClearGroupPermissionsConfig{Group: "Release", Yes: tc.yes, Json: true})
			} else {
				_, err = s.ReplaceGroupPermissions(cli.ReplaceGroupPermissionsConfig{Group: "Release", File: file, Yes: tc.yes, Json: true})
			}
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Equal(t, `[{"key":"run:read"}]`, state)
				require.Empty(t, out.String())
			} else {
				require.NoError(t, err)
				require.Equal(t, `[]`, state)
				require.JSONEq(t, `{"GroupID":"id","Permissions":[]}`, out.String())
				if !tc.yes {
					require.Contains(t, stderr.String(), "complete direct-permission set")
					require.Contains(t, stderr.String(), "Inherited permissions remain unchanged")
				}
			}
		})
	}
}

func TestShowGroupDirectPermissions(t *testing.T) {
	for _, jsonOutput := range []bool{true, false} {
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			body := `{"groups":[{"id":"id","name":"Release"}]}`
			if strings.HasSuffix(req.URL.Path, "/permissions") {
				body = `{"permissions":[{"key":"run:read"},{"key":"vault:manage","resource_id":"vault-id"}]}`
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		var out bytes.Buffer
		s := cli.Service{Config: cli.Config{APIClient: client, Stdout: &out}}
		result, err := s.ShowGroupPermissions(cli.ShowGroupPermissionsConfig{Group: "Release", Json: jsonOutput})
		require.NoError(t, err)
		require.Len(t, result.Permissions, 2)
		if jsonOutput {
			require.JSONEq(t, `{"GroupID":"id","Permissions":[{"Key":"run:read","ResourceID":null},{"Key":"vault:manage","ResourceID":"vault-id"}]}`, out.String())
		} else {
			require.Contains(t, out.String(), "Direct permissions")
			require.Contains(t, out.String(), "inherited permissions unchanged")
			require.Contains(t, out.String(), "vault-id")
			require.Contains(t, out.String(), "(unscoped)")
		}
	}
}
