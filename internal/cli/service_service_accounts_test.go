package cli_test

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestServiceAccountLookup(t *testing.T) {
	for _, selector := range []cli.ServiceAccountSelector{{ServiceAccount: "original-name"}, {DisplayName: "Renamed Bot"}} {
		for _, outcome := range []string{"unique", "ambiguous", "later error", "missing"} {
			t.Run(selector.ServiceAccount+selector.DisplayName+"/"+outcome, func(t *testing.T) {
				var stdout bytes.Buffer
				client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					require.Equal(t, selector.ServiceAccount, req.URL.Query().Get("name"))
					require.Equal(t, selector.DisplayName, req.URL.Query().Get("display_name"))
					require.Equal(t, "100", req.URL.Query().Get("limit"))
					status, body := 200, `{"service_accounts":[{"id":"account","identity_id":"identity","name":"original-name","display_name":"Renamed Bot"}],"pagination":{"next_cursor":"opaque+/=?&"}}`
					if req.URL.Query().Get("cursor") != "" {
						require.Equal(t, "opaque+/=?&", req.URL.Query().Get("cursor"))
						switch outcome {
						case "unique":
							body = `{"service_accounts":[],"pagination":{"next_cursor":null}}`
						case "ambiguous":
							body = `{"service_accounts":[{"id":"another-account"}],"pagination":{"next_cursor":null}}`
						case "later error":
							status, body = 403, `{"error":"Denied"}`
						}
					}
					if outcome == "missing" {
						body = `{"service_accounts":[],"pagination":{"next_cursor":null}}`
					}
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				svc := cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout}}
				result, err := svc.ShowServiceAccount(cli.ShowServiceAccountConfig{ServiceAccountSelector: selector, Json: true})
				switch outcome {
				case "unique":
					require.NoError(t, err)
					require.Equal(t, "original-name", result.Name)
					require.JSONEq(t, `{"ID":"account","IdentityID":"identity","Name":"original-name","DisplayName":"Renamed Bot","Description":null}`, stdout.String())
				case "ambiguous":
					require.ErrorContains(t, err, "more than one")
				case "later error":
					require.ErrorIs(t, err, errors.ErrUnauthenticated)
				case "missing":
					require.ErrorIs(t, err, errors.ErrNotFound)
				}
				if err != nil {
					require.Nil(t, result)
					require.Empty(t, stdout.String())
				}
			})
		}
	}
}

func TestServiceAccountStableIDLookupDoesNotFallBack(t *testing.T) {
	id := "4fc7e888-aaf5-4418-becb-b85f83ed1ee8"
	for _, status := range []int{200, 403, 404} {
		client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/service_accounts/"+id, req.URL.Path)
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"service_account":{"id":"` + id + `","name":"stable"}}`))}, nil
		})
		svc := cli.Service{Config: cli.Config{APIClient: client}}
		result, err := svc.ResolveServiceAccount(cli.ServiceAccountSelector{ServiceAccount: id})
		if status == 200 {
			require.NoError(t, err)
			require.Equal(t, id, result.ID)
		} else {
			require.Error(t, err)
			require.Nil(t, result)
		}
	}
}

func TestServiceAccountsListOnePage(t *testing.T) {
	var stdout bytes.Buffer
	client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "7", req.URL.Query().Get("limit"))
		require.Equal(t, "start+/=", req.URL.Query().Get("cursor"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"service_accounts":[],"pagination":{"next_cursor":"next","limit":7}}`))}, nil
	})
	svc := cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout}}
	result, err := svc.ListServiceAccounts(cli.ListServiceAccountsConfig{Limit: 7, Cursor: "start+/=", Json: true})
	require.NoError(t, err)
	require.Empty(t, result.ServiceAccounts)
	require.JSONEq(t, `{"ServiceAccounts":[],"NextCursor":"next","Limit":7}`, stdout.String())
	for _, limit := range []int{-1, 101} {
		_, err := svc.ListServiceAccounts(cli.ListServiceAccountsConfig{Limit: limit})
		require.ErrorContains(t, err, "limit")
	}
}

func TestServiceAccountDeactivationSafety(t *testing.T) {
	for _, tc := range []struct {
		name             string
		tty, yes         bool
		input, wantError string
		deletes          int
	}{
		{"noninteractive", false, false, "", "--yes", 0},
		{"declined", true, false, "n\n", "aborted", 0},
		{"EOF", true, false, "", "no input", 0},
		{"confirmed", true, false, "yes\n", "", 1},
		{"yes", false, true, "", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			deletes := 0
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.Method == "DELETE" {
					deletes++
					require.Equal(t, "/mint/api/service_accounts/account", req.URL.Path)
					return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"service_accounts":[{"id":"account","name":"stable","display_name":"Label"}],"pagination":{}}`))}, nil
			})
			svc := cli.Service{Config: cli.Config{APIClient: client, Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(tc.input), StderrIsTTY: tc.tty}}
			result, err := svc.DeactivateServiceAccount(cli.DeactivateServiceAccountConfig{ServiceAccountSelector: cli.ServiceAccountSelector{ServiceAccount: "stable"}, Yes: tc.yes, Json: true})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, result)
				require.Empty(t, stdout.String())
			} else {
				require.NoError(t, err)
				require.Equal(t, "account", result.ID)
				require.Contains(t, stdout.String(), `"ID":"account"`)
			}
			require.Equal(t, tc.deletes, deletes)
			if tc.tty && !tc.yes {
				require.Contains(t, stderr.String(), "revokes its access tokens")
				require.Contains(t, stderr.String(), "ends its group memberships")
				require.Contains(t, stderr.String(), `"stable" (account)`)
			}
		})
	}
}
