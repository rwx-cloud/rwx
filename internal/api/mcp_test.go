package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestClient_McpGetRunTestFailures(t *testing.T) {
	t.Run("makes correct API call with valid run URLs", func(t *testing.T) {
		expectedResponse := &api.McpTextResult{
			Type: "text",
			Text: "Failed tests:\n- spec/models/user_spec.rb\n- spec/controllers/api_controller_spec.rb",
		}
		responseBytes, _ := json.Marshal(expectedResponse)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/mcp/get_run_test_failures", req.URL.Path)
			require.Equal(t, "POST", req.Method)
			require.Equal(t, "application/json", req.Header.Get("Content-Type"))
			require.Equal(t, "application/json", req.Header.Get("Accept"))
			require.Empty(t, req.Header.Get("X-RWX-Prompt-Presentation"))

			var requestBody api.McpGetRunTestFailuresRequest
			bodyBytes, _ := io.ReadAll(req.Body)
			err := json.Unmarshal(bodyBytes, &requestBody)
			require.NoError(t, err)
			require.Equal(t, []string{"https://cloud.rwx.com/mint/org/runs/123", "https://cloud.rwx.com/mint/org/runs/456"}, requestBody.RunUrls)

			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(responseBytes)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}

		client := api.NewClientWithRoundTrip(roundTrip)
		result, err := client.McpGetRunTestFailures(api.McpGetRunTestFailuresRequest{
			RunUrls: []string{"https://cloud.rwx.com/mint/org/runs/123", "https://cloud.rwx.com/mint/org/runs/456"},
		})

		require.NoError(t, err)
		require.Equal(t, expectedResponse, result)
	})

	t.Run("handles HTTP error responses", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "404 Not Found",
				StatusCode: 404,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Run not found"}`))),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}

		client := api.NewClientWithRoundTrip(roundTrip)
		_, err := client.McpGetRunTestFailures(api.McpGetRunTestFailuresRequest{
			RunUrls: []string{"https://cloud.rwx.com/mint/org/runs/nonexistent"},
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("validates response type field", func(t *testing.T) {
		invalidResponse := map[string]any{
			"type": "invalid_type",
			"text": "some text",
		}
		responseBytes, _ := json.Marshal(invalidResponse)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(responseBytes)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}

		client := api.NewClientWithRoundTrip(roundTrip)
		_, err := client.McpGetRunTestFailures(api.McpGetRunTestFailuresRequest{
			RunUrls: []string{"https://cloud.rwx.com/mint/org/runs/123"},
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), `unexpected response type "invalid_type"`)
	})
}
