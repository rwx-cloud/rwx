package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_InitiateRun(t *testing.T) {
	t.Run("prefixes the endpoint with the base path and parses camelcase responses", func(t *testing.T) {
		body := struct {
			RunID            string   `json:"runId"`
			RunURL           string   `json:"runUrl"`
			TargetedTaskKeys []string `json:"targetedTaskKeys"`
			DefinitionPath   string   `json:"definitionPath"`
		}{
			RunID:            "123",
			RunURL:           "https://cloud.rwx.com/mint/org/runs/123",
			TargetedTaskKeys: []string{},
			DefinitionPath:   "foo",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/runs", req.URL.Path)
			return &http.Response{
				Status:     "201 Created",
				StatusCode: 201,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		initRunConfig := api.InitiateRunConfig{
			InitializationParameters: []api.InitializationParameter{},
			TaskDefinitions: []api.RwxDirectoryEntry{
				{Path: "foo", FileContents: "echo 'bar'", Permissions: 0o644, Type: "file"},
			},
			TargetedTaskKeys: []string{},
			UseCache:         false,
		}

		result, err := c.InitiateRun(initRunConfig)
		require.NoError(t, err)
		require.Equal(t, "123", result.RunID)
	})

	t.Run("prefixes the endpoint with the base path and parses snakecase responses", func(t *testing.T) {
		body := struct {
			RunID            string   `json:"run_id"`
			RunURL           string   `json:"run_url"`
			TargetedTaskKeys []string `json:"targeted_task_keys"`
			DefinitionPath   string   `json:"definition_path"`
		}{
			RunID:            "123",
			RunURL:           "https://cloud.rwx.com/mint/org/runs/123",
			TargetedTaskKeys: []string{},
			DefinitionPath:   "foo",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/runs", req.URL.Path)
			return &http.Response{
				Status:     "201 Created",
				StatusCode: 201,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		initRunConfig := api.InitiateRunConfig{
			InitializationParameters: []api.InitializationParameter{},
			TaskDefinitions: []api.RwxDirectoryEntry{
				{Path: "foo", FileContents: "echo 'bar'", Permissions: 0o644, Type: "file"},
			},
			TargetedTaskKeys: []string{},
			UseCache:         false,
		}

		result, err := c.InitiateRun(initRunConfig)
		require.NoError(t, err)
		require.Equal(t, "123", result.RunID)
	})
}

func TestAPIClient_ObtainAuthCode(t *testing.T) {
	t.Run("builds the request", func(t *testing.T) {
		body := struct {
			AuthorizationUrl string `json:"authorization_url"`
			TokenUrl         string `json:"token_url"`
		}{
			AuthorizationUrl: "https://cloud.rwx.com/_/auth/code?code=foobar",
			TokenUrl:         "https://cloud.rwx.com/api/auth/codes/code-uuid/token",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/api/auth/codes", req.URL.Path)
			return &http.Response{
				Status:     "201 Created",
				StatusCode: 201,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		obtainAuthCodeConfig := api.ObtainAuthCodeConfig{
			Code: api.ObtainAuthCodeCode{
				DeviceName: "some-device",
			},
		}

		_, err := c.ObtainAuthCode(obtainAuthCodeConfig)
		require.NoError(t, err)
	})
}

func TestAPIClient_AcquireToken(t *testing.T) {
	t.Run("builds the request using the supplied url", func(t *testing.T) {
		body := struct {
			State string `json:"state"`
			Token string `json:"token"`
		}{
			State: "authorized",
			Token: "some-token",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			expected, err := url.Parse("https://cloud.rwx.com/api/auth/codes/some-uuid/token")
			require.NoError(t, err)
			require.Equal(t, expected, req.URL)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.AcquireToken("https://cloud.rwx.com/api/auth/codes/some-uuid/token")
		require.NoError(t, err)
	})
}

func TestAPIClient_Whoami(t *testing.T) {
	t.Run("makes the request", func(t *testing.T) {
		email := "some-email@example.com"
		body := struct {
			OrganizationSlug string  `json:"organization_slug"`
			TokenKind        string  `json:"token_kind"`
			UserEmail        *string `json:"user_email,omitempty"`
		}{
			OrganizationSlug: "some-org",
			TokenKind:        "personal_access_token",
			UserEmail:        &email,
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/api/auth/whoami", req.URL.Path)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.Whoami()
		require.NoError(t, err)
	})
}

func TestAPIClient_SetSecretsInVault(t *testing.T) {
	t.Run("makes the request", func(t *testing.T) {
		body := api.SetSecretsInVaultConfig{
			VaultName: "default",
			Secrets:   []api.Secret{{Name: "ABC", Secret: "123"}},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/vaults/secrets", req.URL.Path)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.SetSecretsInVault(body)
		require.NoError(t, err)
	})
}

func TestAPIClient_InitiateDispatch(t *testing.T) {
	t.Run("builds the request and parses the response", func(t *testing.T) {
		body := struct {
			DispatchId string `json:"dispatch_id"`
		}{
			DispatchId: "dispatch-123",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/runs/dispatches", req.URL.Path)
			require.Equal(t, http.MethodPost, req.Method)
			return &http.Response{
				Status:     "201 Created",
				StatusCode: 201,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		dispatchConfig := api.InitiateDispatchConfig{
			DispatchKey: "test-dispatch-key",
			Params:      map[string]string{"key1": "value1"},
			Ref:         "main",
			Title:       "Test Dispatch",
		}

		result, err := c.InitiateDispatch(dispatchConfig)
		require.NoError(t, err)
		require.Equal(t, "dispatch-123", result.DispatchId)
	})
}

func TestAPIClient_GetDispatch(t *testing.T) {
	t.Run("builds the request and parses the response", func(t *testing.T) {
		body := struct {
			Status string               `json:"status"`
			Error  string               `json:"error"`
			Runs   []api.GetDispatchRun `json:"runs"`
		}{
			Status: "ready",
			Error:  "",
			Runs:   []api.GetDispatchRun{{RunID: "run-123", RunUrl: "https://example.com/run-123"}},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/runs/dispatches/dispatch-123", req.URL.Path)
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		dispatchConfig := api.GetDispatchConfig{
			DispatchId: "dispatch-123",
		}

		result, err := c.GetDispatch(dispatchConfig)
		require.NoError(t, err)
		require.Equal(t, "ready", result.Status)
		require.Len(t, result.Runs, 1)
		require.Equal(t, "run-123", result.Runs[0].RunID)
		require.Equal(t, "https://example.com/run-123", result.Runs[0].RunUrl)
	})
}

func TestAPIClient_GetDefaultBase(t *testing.T) {
	t.Run("builds the request and parses the response", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/base/default", req.URL.Path)
			require.Equal(t, http.MethodGet, req.Method)

			body := `{"image": "ubuntu:24.04", "config": "rwx/base 1.0.0", "arch": "x86_64"}`
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte(body))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetDefaultBase()
		require.NoError(t, err)
		require.Equal(t, "ubuntu:24.04", result.Image)
		require.Equal(t, "rwx/base 1.0.0", result.Config)
		require.Equal(t, "x86_64", result.Arch)
	})
}

func TestAPIClient_StartImagePush(t *testing.T) {
	t.Run("builds the request and parses the response", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/images/pushes", req.URL.Path)
			require.Equal(t, http.MethodPost, req.Method)
			reqBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Contains(t, string(reqBody), "task-123")
			require.Contains(t, string(reqBody), "myuser")
			require.Contains(t, string(reqBody), "mypass")
			require.Contains(t, string(reqBody), "dockerhub.io")
			require.Contains(t, string(reqBody), "myimage")
			require.Contains(t, string(reqBody), "latest")
			require.Contains(t, string(reqBody), "other")

			body := `{"push_id": "push-123"}`
			return &http.Response{
				Status:     "202 Accepted",
				StatusCode: 202,
				Body:       io.NopCloser(bytes.NewReader([]byte(body))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		pushConfig := api.StartImagePushConfig{
			TaskID: "task-123",
			Credentials: api.StartImagePushConfigCredentials{
				Username: "myuser",
				Password: "mypass",
			},
			Image: api.StartImagePushConfigImage{
				Registry:   "dockerhub.io",
				Repository: "myimage",
				Tags:       []string{"latest", "other"},
			},
		}

		result, err := c.StartImagePush(pushConfig)
		require.NoError(t, err)
		require.Equal(t, "push-123", result.PushID)
	})
}

func TestAPIClient_ImagePushStatus(t *testing.T) {
	t.Run("builds the request and parses the response", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/images/pushes/abc123", req.URL.Path)
			require.Equal(t, http.MethodGet, req.Method)

			body := `{"status": "in_progress"}`
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte(body))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.ImagePushStatus("abc123")
		require.NoError(t, err)
		require.Equal(t, "in_progress", result.Status)
	})
}

func TestAPIClient_TaskIDStatus(t *testing.T) {
	t.Run("builds the request and parses the response", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/tasks/abc123/status", req.URL.Path)
			require.Equal(t, http.MethodGet, req.Method)

			body := `{"polling": {"completed": true}}`
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte(body))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.TaskIDStatus(api.TaskIDStatusConfig{TaskID: "abc123"})
		require.NoError(t, err)
		require.True(t, result.Polling.Completed)
	})
}

func TestAPIClient_GetLogDownloadRequest(t *testing.T) {
	t.Run("builds the request and parses the response without contents", func(t *testing.T) {
		body := struct {
			URL      string `json:"url"`
			Token    string `json:"token"`
			Filename string `json:"filename"`
		}{
			URL:      "https://example.com/logs/download",
			Token:    "jwt-token-123",
			Filename: "task-123-logs.log",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/log_download", req.URL.Path)
			require.Equal(t, "task-123", req.URL.Query().Get("id"))
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetLogDownloadRequest("task-123")
		require.NoError(t, err)
		require.Equal(t, "https://example.com/logs/download", result.URL)
		require.Equal(t, "jwt-token-123", result.Token)
		require.Equal(t, "task-123-logs.log", result.Filename)
		require.Equal(t, "", result.Contents)
	})

	t.Run("builds the request and parses the response with contents", func(t *testing.T) {
		body := struct {
			URL      string `json:"url"`
			Token    string `json:"token"`
			Filename string `json:"filename"`
			Contents string `json:"contents"`
		}{
			URL:      "https://example.com/logs/download",
			Token:    "jwt-token-123",
			Filename: "task-123-logs.zip",
			Contents: `{"key":"value"}`,
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/log_download", req.URL.Path)
			require.Equal(t, "task-123", req.URL.Query().Get("id"))
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetLogDownloadRequest("task-123")
		require.NoError(t, err)
		require.Equal(t, "https://example.com/logs/download", result.URL)
		require.Equal(t, "jwt-token-123", result.Token)
		require.Equal(t, "task-123-logs.zip", result.Filename)
		require.Equal(t, `{"key":"value"}`, result.Contents)
	})

	t.Run("handles 404 not found", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/log_download", req.URL.Path)
			require.Equal(t, "task-999", req.URL.Query().Get("id"))
			return &http.Response{
				Status:     "404 Not Found",
				StatusCode: 404,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Task not found"}`))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.GetLogDownloadRequest("task-999")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})
}

func TestAPIClient_DownloadLogs(t *testing.T) {
	t.Run("makes POST request with form data", func(t *testing.T) {
		zipContents := []byte("PK\x03\x04\x14\x00\x08\x00\x08\x00")
		contents := `{"key":"value"}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			err := r.ParseForm()
			require.NoError(t, err)
			require.Equal(t, "jwt-token-123", r.Form.Get("token"))
			require.Equal(t, "task-123-logs.zip", r.Form.Get("filename"))
			require.Equal(t, contents, r.Form.Get("contents"))

			w.WriteHeader(http.StatusOK)
			_, writeErr := w.Write(zipContents)
			require.NoError(t, writeErr)
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		result, err := c.DownloadLogs(api.LogDownloadRequestResult{
			URL:      server.URL,
			Token:    "jwt-token-123",
			Filename: "task-123-logs.zip",
			Contents: contents,
		})

		require.NoError(t, err)
		require.Equal(t, zipContents, result)
	})

	t.Run("returns error on 4xx response without retry", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attemptCount++
			w.WriteHeader(http.StatusBadRequest)
			_, err := w.Write([]byte(`{"error": "Bad request"}`))
			require.NoError(t, err)
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		_, err := c.DownloadLogs(api.LogDownloadRequestResult{
			URL:      server.URL,
			Token:    "token",
			Filename: "logs.log",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "Bad request")
		require.Equal(t, 1, attemptCount, "should not retry on 4xx errors")
	})

	t.Run("returns error on request failure after retries", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		serverURL := server.URL
		server.Close() // Close immediately so connections fail

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		startTime := time.Now()
		// Use 2 seconds for faster test execution
		_, err := c.DownloadLogs(api.LogDownloadRequestResult{
			URL:      serverURL,
			Token:    "token",
			Filename: "logs.log",
		}, 2)

		elapsed := time.Since(startTime)
		require.Error(t, err)
		require.Contains(t, err.Error(), "HTTP request failed")
		require.Contains(t, err.Error(), "failed after")
		require.Contains(t, err.Error(), "attempts")
		require.Greater(t, elapsed, 1*time.Second, "should retry for approximately 2 seconds")
		require.Less(t, elapsed, 4*time.Second, "should not exceed 2 seconds significantly (allowing for backoff delays)")
	})

	t.Run("retries on 5xx errors and succeeds", func(t *testing.T) {
		attemptCount := 0
		logContents := []byte("log data")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attemptCount++
			if attemptCount < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error": "Service temporarily unavailable"}`))
				return
			}
			// Third attempt succeeds
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(logContents)
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		// Use 5 seconds for faster test execution
		result, err := c.DownloadLogs(api.LogDownloadRequestResult{
			URL:      server.URL,
			Token:    "token",
			Filename: "logs.log",
		}, 5)

		require.NoError(t, err)
		require.Equal(t, logContents, result)
		require.Equal(t, 3, attemptCount)
	})

	t.Run("does not retry on 4xx errors", func(t *testing.T) {
		attemptCount := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attemptCount++
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "Not found"}`))
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		_, err := c.DownloadLogs(api.LogDownloadRequestResult{
			URL:      server.URL,
			Token:    "token",
			Filename: "logs.log",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "Not found")
		require.Equal(t, 1, attemptCount, "should not retry on 404")
	})
}

func TestAPIClient_GetArtifactDownloadRequest(t *testing.T) {
	t.Run("builds the request and parses the response", func(t *testing.T) {
		body := struct {
			URL         string `json:"url"`
			Filename    string `json:"filename"`
			SizeInBytes int64  `json:"size_in_bytes"`
			Kind        string `json:"kind"`
			Key         string `json:"key"`
		}{
			URL:         "https://s3.example.com/artifacts/abc123",
			Filename:    "task-123-my-artifact.tar",
			SizeInBytes: 1024,
			Kind:        "file",
			Key:         "my-artifact",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/artifact_download", req.URL.Path)
			require.Equal(t, "task-123", req.URL.Query().Get("task_id"))
			require.Equal(t, "my-artifact", req.URL.Query().Get("key"))
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetArtifactDownloadRequest("task-123", "my-artifact")
		require.NoError(t, err)
		require.Equal(t, "https://s3.example.com/artifacts/abc123", result.URL)
		require.Equal(t, "task-123-my-artifact.tar", result.Filename)
		require.Equal(t, int64(1024), result.SizeInBytes)
		require.Equal(t, "file", result.Kind)
		require.Equal(t, "my-artifact", result.Key)
	})

	t.Run("builds the request with directory kind", func(t *testing.T) {
		body := struct {
			URL         string `json:"url"`
			Filename    string `json:"filename"`
			SizeInBytes int64  `json:"size_in_bytes"`
			Kind        string `json:"kind"`
			Key         string `json:"key"`
		}{
			URL:         "https://s3.example.com/artifacts/def456",
			Filename:    "task-456~my-dir.tar",
			SizeInBytes: 4096,
			Kind:        "directory",
			Key:         "my-dir",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/artifact_download", req.URL.Path)
			require.Equal(t, "task-456", req.URL.Query().Get("task_id"))
			require.Equal(t, "my-dir", req.URL.Query().Get("key"))
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetArtifactDownloadRequest("task-456", "my-dir")
		require.NoError(t, err)
		require.Equal(t, "directory", result.Kind)
	})

	t.Run("handles 404 not found", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/artifact_download", req.URL.Path)
			require.Equal(t, "task-999", req.URL.Query().Get("task_id"))
			require.Equal(t, "missing", req.URL.Query().Get("key"))
			return &http.Response{
				Status:     "404 Not Found",
				StatusCode: 404,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Artifact not found"}`))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.GetArtifactDownloadRequest("task-999", "missing")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("handles special characters in artifact key", func(t *testing.T) {
		body := struct {
			URL      string `json:"url"`
			Filename string `json:"filename"`
			Kind     string `json:"kind"`
			Key      string `json:"key"`
		}{
			URL:      "https://example.com/artifact",
			Filename: "file.tar",
			Kind:     "file",
			Key:      "my-artifact-v1.2.3",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/artifact_download", req.URL.Path)
			require.Equal(t, "my-artifact-v1.2.3", req.URL.Query().Get("key"))
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.GetArtifactDownloadRequest("task-123", "my-artifact-v1.2.3")
		require.NoError(t, err)
	})
}

func TestAPIClient_GetArtifactDownloadRequestByTaskKey(t *testing.T) {
	t.Run("sends run_id, task_key, and key as query params", func(t *testing.T) {
		body := struct {
			URL         string `json:"url"`
			Filename    string `json:"filename"`
			SizeInBytes int64  `json:"size_in_bytes"`
			Kind        string `json:"kind"`
			Key         string `json:"key"`
		}{
			URL:         "https://s3.example.com/artifacts/abc123",
			Filename:    "build-task-123-my-artifact.tar",
			SizeInBytes: 1024,
			Kind:        "file",
			Key:         "my-artifact",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/artifact_download", req.URL.Path)
			require.Equal(t, "run-123", req.URL.Query().Get("run_id"))
			require.Equal(t, "build", req.URL.Query().Get("task_key"))
			require.Equal(t, "my-artifact", req.URL.Query().Get("key"))
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetArtifactDownloadRequestByTaskKey("run-123", "build", "my-artifact")
		require.NoError(t, err)
		require.Equal(t, "https://s3.example.com/artifacts/abc123", result.URL)
		require.Equal(t, "file", result.Kind)
		require.Equal(t, "my-artifact", result.Key)
	})
}

func TestAPIClient_GetAllArtifactDownloadRequests(t *testing.T) {
	t.Run("parses array response", func(t *testing.T) {
		body := []struct {
			URL         string `json:"url"`
			Filename    string `json:"filename"`
			SizeInBytes int64  `json:"size_in_bytes"`
			Kind        string `json:"kind"`
			Key         string `json:"key"`
		}{
			{
				URL:         "https://s3.example.com/artifacts/abc123",
				Filename:    "task-123~artifact-a.tar",
				SizeInBytes: 1024,
				Kind:        "file",
				Key:         "artifact-a",
			},
			{
				URL:         "https://s3.example.com/artifacts/def456",
				Filename:    "task-123~artifact-b.tar",
				SizeInBytes: 2048,
				Kind:        "directory",
				Key:         "artifact-b",
			},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/artifact_downloads", req.URL.Path)
			require.Equal(t, "task-123", req.URL.Query().Get("task_id"))
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		results, err := c.GetAllArtifactDownloadRequests("task-123")
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.Equal(t, "artifact-a", results[0].Key)
		require.Equal(t, "file", results[0].Kind)
		require.Equal(t, "artifact-b", results[1].Key)
		require.Equal(t, "directory", results[1].Kind)
	})

	t.Run("handles 404 not found", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "404 Not Found",
				StatusCode: 404,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Task not found"}`))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.GetAllArtifactDownloadRequests("task-999")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("handles empty array", func(t *testing.T) {
		bodyBytes, _ := json.Marshal([]struct{}{})

		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		results, err := c.GetAllArtifactDownloadRequests("task-123")
		require.NoError(t, err)
		require.Empty(t, results)
	})
}

func TestAPIClient_RunStatus(t *testing.T) {
	t.Run("parses the response when run is in progress", func(t *testing.T) {
		backoffMs := 2000
		body := struct {
			Status  *api.RunStatus    `json:"run_status"`
			RunID   string            `json:"run_id"`
			Polling api.PollingResult `json:"polling"`
		}{
			Status:  &api.RunStatus{Result: ""},
			RunID:   "run-123",
			Polling: api.PollingResult{Completed: false, BackoffMs: &backoffMs},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/runs/run-123/status", req.URL.Path)
			require.Equal(t, "true", req.URL.Query().Get("fail_fast"))
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.RunStatus(api.RunStatusConfig{RunID: "run-123", FailFast: true})
		require.NoError(t, err)
		require.NotNil(t, result.Status)
		require.Equal(t, "", result.Status.Result)
		require.Equal(t, "run-123", result.RunID)
		require.False(t, result.Polling.Completed)
		require.NotNil(t, result.Polling.BackoffMs)
		require.Equal(t, 2000, *result.Polling.BackoffMs)
	})

	t.Run("sends fail_fast as false by default", func(t *testing.T) {
		body := struct {
			Status  *api.RunStatus    `json:"run_status"`
			Polling api.PollingResult `json:"polling"`
		}{
			Status:  &api.RunStatus{Result: "succeeded"},
			Polling: api.PollingResult{Completed: true},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "false", req.URL.Query().Get("fail_fast"))
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.RunStatus(api.RunStatusConfig{RunID: "run-123"})
		require.NoError(t, err)
	})

	t.Run("parses the response when run is completed", func(t *testing.T) {
		body := struct {
			Status  *api.RunStatus    `json:"run_status"`
			RunID   string            `json:"run_id"`
			Polling api.PollingResult `json:"polling"`
		}{
			Status:  &api.RunStatus{Result: "succeeded"},
			RunID:   "run-456",
			Polling: api.PollingResult{Completed: true},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/runs/run-456/status", req.URL.Path)
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.RunStatus(api.RunStatusConfig{RunID: "run-456"})
		require.NoError(t, err)
		require.NotNil(t, result.Status)
		require.Equal(t, "succeeded", result.Status.Result)
		require.Equal(t, "run-456", result.RunID)
		require.True(t, result.Polling.Completed)
		require.Nil(t, result.Polling.BackoffMs)
	})

	t.Run("parses the response when run is not found", func(t *testing.T) {
		body := struct {
			Status  *api.RunStatus    `json:"run_status"`
			Polling api.PollingResult `json:"polling"`
		}{
			Status:  nil,
			Polling: api.PollingResult{Completed: true},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.RunStatus(api.RunStatusConfig{RunID: "nonexistent"})
		require.NoError(t, err)
		require.Nil(t, result.Status)
		require.True(t, result.Polling.Completed)
	})
}

func TestAPIClient_GetSandboxConnectionInfo(t *testing.T) {
	t.Run("returns connection info when sandbox is ready", func(t *testing.T) {
		backoffMs := 1000
		body := struct {
			Sandboxable    bool              `json:"sandboxable"`
			Address        string            `json:"address"`
			PublicHostKey  string            `json:"public_host_key"`
			PrivateUserKey string            `json:"private_user_key"`
			Polling        api.PollingResult `json:"polling"`
		}{
			Sandboxable:    true,
			Address:        "192.168.1.1:22",
			PublicHostKey:  "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5...",
			PrivateUserKey: "-----BEGIN OPENSSH PRIVATE KEY-----...",
			Polling:        api.PollingResult{Completed: false, BackoffMs: &backoffMs},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/sandbox_connection_info", req.URL.Path)
			require.Equal(t, "run-123", req.URL.Query().Get("sandbox_key"))
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetSandboxConnectionInfo("run-123", "")
		require.NoError(t, err)
		require.True(t, result.Sandboxable)
		require.Equal(t, "192.168.1.1:22", result.Address)
		require.Equal(t, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5...", result.PublicHostKey)
		require.Equal(t, "-----BEGIN OPENSSH PRIVATE KEY-----...", result.PrivateUserKey)
		require.False(t, result.Polling.Completed)
	})

	t.Run("returns connection info when sandbox is not ready yet", func(t *testing.T) {
		backoffMs := 2000
		body := struct {
			Sandboxable bool              `json:"sandboxable"`
			Polling     api.PollingResult `json:"polling"`
		}{
			Sandboxable: false,
			Polling:     api.PollingResult{Completed: false, BackoffMs: &backoffMs},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetSandboxConnectionInfo("run-456", "")
		require.NoError(t, err)
		require.False(t, result.Sandboxable)
		require.NotNil(t, result.Polling.BackoffMs)
		require.Equal(t, 2000, *result.Polling.BackoffMs)
	})

	t.Run("returns error on 404", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "404 Not Found",
				StatusCode: 404,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Run not found"}`))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.GetSandboxConnectionInfo("nonexistent", "")
		require.Error(t, err)
	})

	t.Run("returns error on 410 gone", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "410 Gone",
				StatusCode: 410,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Run has ended"}`))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.GetSandboxConnectionInfo("ended-run", "")
		require.Error(t, err)
	})

	t.Run("returns error when runID is empty", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			t.Fatal("should not make request")
			return nil, nil
		})

		_, err := c.GetSandboxConnectionInfo("", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing runID")
	})

	t.Run("sets Authorization header when scoped token is provided", func(t *testing.T) {
		backoffMs := 1000
		body := struct {
			Sandboxable bool              `json:"sandboxable"`
			Polling     api.PollingResult `json:"polling"`
		}{
			Sandboxable: true,
			Polling:     api.PollingResult{Completed: false, BackoffMs: &backoffMs},
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/sandbox_connection_info", req.URL.Path)
			require.Equal(t, "run-123", req.URL.Query().Get("sandbox_key"))
			require.Equal(t, "Bearer scoped-token-abc", req.Header.Get("Authorization"))
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetSandboxConnectionInfo("run-123", "scoped-token-abc")
		require.NoError(t, err)
		require.True(t, result.Sandboxable)
	})
}

func TestAPIClient_CreateSandboxToken(t *testing.T) {
	t.Run("creates a sandbox token successfully", func(t *testing.T) {
		body := struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expires_at"`
			RunID     string `json:"run_id"`
		}{
			Token:     "scoped-token-xyz",
			ExpiresAt: "2024-12-31T23:59:59Z",
			RunID:     "run-123",
		}
		bodyBytes, _ := json.Marshal(body)

		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/sandbox_tokens", req.URL.Path)
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "application/json", req.Header.Get("Content-Type"))

			// Verify request body
			var reqBody api.CreateSandboxTokenConfig
			err := json.NewDecoder(req.Body).Decode(&reqBody)
			require.NoError(t, err)
			require.Equal(t, "run-123", reqBody.RunID)

			return &http.Response{
				Status:     "201 Created",
				StatusCode: 201,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.CreateSandboxToken(api.CreateSandboxTokenConfig{
			RunID: "run-123",
		})
		require.NoError(t, err)
		require.Equal(t, "scoped-token-xyz", result.Token)
		require.Equal(t, "2024-12-31T23:59:59Z", result.ExpiresAt)
		require.Equal(t, "run-123", result.RunID)
	})

	t.Run("returns error on failure", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "403 Forbidden",
				StatusCode: 403,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Not authorized to create token for this run"}`))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.CreateSandboxToken(api.CreateSandboxTokenConfig{
			RunID: "run-123",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Not authorized")
	})
}

func TestAPIClient_GetRunPrompt(t *testing.T) {
	t.Run("builds the request and returns the prompt text", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "/mint/api/runs/run-123/prompt", req.URL.Path)
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "text/plain", req.Header.Get("Accept"))
			return &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte("some prompt text"))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.GetRunPrompt("run-123")
		require.NoError(t, err)
		require.NotEmpty(t, result)
	})

	t.Run("handles 404 not found", func(t *testing.T) {
		roundTrip := func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "404 Not Found",
				StatusCode: 404,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Run not found"}`))),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		_, err := c.GetRunPrompt("nonexistent")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})
}

func TestAPIClient_DownloadArtifact(t *testing.T) {
	t.Run("makes GET request to presigned URL", func(t *testing.T) {
		artifactContents := []byte("artifact binary data")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "application/octet-stream", r.Header.Get("Accept"))

			w.WriteHeader(http.StatusOK)
			_, writeErr := w.Write(artifactContents)
			require.NoError(t, writeErr)
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		result, err := c.DownloadArtifact(api.ArtifactDownloadRequestResult{
			URL:      server.URL,
			Filename: "artifact.tar",
			Kind:     "file",
			Key:      "my-artifact",
		})

		require.NoError(t, err)
		require.Equal(t, artifactContents, result)
	})

	t.Run("returns error on 4xx response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, err := w.Write([]byte(`<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`))
			require.NoError(t, err)
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		_, err := c.DownloadArtifact(api.ArtifactDownloadRequestResult{
			URL:      server.URL,
			Filename: "artifact.tar",
			Kind:     "file",
			Key:      "my-artifact",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "Unable to download artifact")
	})

	t.Run("returns error on 404 from S3", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, err := w.Write([]byte(`<?xml version="1.0"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`))
			require.NoError(t, err)
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		_, err := c.DownloadArtifact(api.ArtifactDownloadRequestResult{
			URL:      server.URL,
			Filename: "artifact.tar",
			Kind:     "file",
			Key:      "my-artifact",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
	})

	t.Run("returns error on 403 errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`))
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		_, err := c.DownloadArtifact(api.ArtifactDownloadRequestResult{
			URL:      server.URL,
			Filename: "artifact.tar",
			Kind:     "file",
			Key:      "my-artifact",
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "Unable to download artifact")
	})

	t.Run("handles large artifact download", func(t *testing.T) {
		// Create a 1MB artifact
		largeContent := make([]byte, 1024*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, writeErr := w.Write(largeContent)
			require.NoError(t, writeErr)
		}))
		defer server.Close()

		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		})

		result, err := c.DownloadArtifact(api.ArtifactDownloadRequestResult{
			URL:      server.URL,
			Filename: "large-artifact.tar",
			Kind:     "directory",
			Key:      "large-artifact",
		})

		require.NoError(t, err)
		require.Equal(t, largeContent, result)
		require.Equal(t, 1024*1024, len(result))
	})
}
