package api_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	internalErrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestAppRequests(t *testing.T) {
	for _, action := range []string{"disable", "enable"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "/mint/api/app_endpoints/"+action, req.URL.String())
				require.Equal(t, "application/json", req.Header.Get("Accept"))
				require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"endpoint":"pr/123?\" name"}`, string(body))
				return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			operation := client.DisableApp
			if action == "enable" {
				operation = client.EnableApp
			}
			for range 2 {
				require.NoError(t, operation(`pr/123?" name`))
			}
			require.Equal(t, 2, calls)
		})
	}
}

func TestAppErrors(t *testing.T) {
	for _, action := range []string{"disable", "enable"} {
		for _, tc := range []struct {
			name     string
			status   int
			body     string
			want     string
			sentinel error
		}{
			{"not found", 404, `{"error_messages":[{"message":"App endpoint \"pr-123\" not found."}]}`, `App endpoint "pr-123" not found.`, api.ErrNotFound},
			{"blank name", 400, `{"error_messages":[{"message":"App endpoint name is required."}]}`, "App endpoint name is required.", nil},
			{"invalid name type", 400, `{"error_messages":[{"message":"App endpoint name must be a string."}]}`, "App endpoint name must be a string.", nil},
			{"unauthorized", 401, `{"error":"Unauthorized"}`, "Unauthorized", internalErrors.ErrUnauthenticated},
			{"forbidden", 403, `{"error_messages":[{"message":"You do not have permission to perform this action. It requires the \"run:manage\" permission."}],"required_permission":"run:manage"}`, `It requires the "run:manage" permission.`, internalErrors.ErrUnauthenticated},
			{"server error", 500, `{}`, "500 Internal Server Error", internalErrors.ErrInternalServerError},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: tc.status, Status: "500 Internal Server Error", Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				})
				operation := client.DisableApp
				if action == "enable" {
					operation = client.EnableApp
				}
				err := operation("pr-123")
				require.ErrorContains(t, err, tc.want)
				if tc.sentinel != nil {
					require.ErrorIs(t, err, tc.sentinel)
				}
				if tc.status == http.StatusUnauthorized {
					require.ErrorContains(t, err, "rwx login")
				}
			})
		}
		t.Run(action+"/transport error", func(t *testing.T) {
			offline := errors.New("offline")
			client := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				return nil, offline
			})
			operation := client.DisableApp
			if action == "enable" {
				operation = client.EnableApp
			}
			err := operation("pr-123")
			require.ErrorContains(t, err, "HTTP request failed: offline")
			require.ErrorIs(t, err, offline)
		})
	}
}
