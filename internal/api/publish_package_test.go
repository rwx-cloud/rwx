package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_PublishPackage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"success", 200, `{}`, ""},
		{"version conflict", 400, `{"error":"acme/tool 1.0.0 has already been published. Bump the version number according to semver."}`, "acme/tool 1.0.0 has already been published"},
		{"unauthorized", 401, `{"error":"Unauthorized"}`, "Unauthorized"},
		{"forbidden", 403, `{"error_messages":[{"message":"Requires package:publish permission"}]}`, "Requires package:publish permission"},
		{"not found", 404, "", "404 Not Found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "/mint/api/leaves/publish", req.URL.Path)
				require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				require.Equal(t, "application/json", req.Header.Get("Accept"))
				var body map[string]string
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				require.Equal(t, map[string]string{"digest": "abc123"}, body)
				return &http.Response{
					StatusCode: tc.status,
					Status:     fmt.Sprintf("%d %s", tc.status, http.StatusText(tc.status)),
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}, nil
			})
			err := c.PublishPackage("abc123")
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}
