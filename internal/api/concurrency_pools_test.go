package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_ListConcurrencyPools(t *testing.T) {
	var captured *http.Request
	c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		captured = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"concurrency_pools":[{
					"id":"rwx-cloud/rwx:main",
					"status":"active",
					"paused_at":null,
					"queue_size":2,
					"usage_count_last_7_days":14,
					"last_requested_at":"2026-10-05T18:00:00Z",
					"latest_configuration":{"capacity":1,"on_overflow":"cancel-waiting"}
				}],
				"pagination":{"next_cursor":"next-page","limit":1}
			}`)),
		}, nil
	})

	result, err := c.ListConcurrencyPools(api.ListConcurrencyPoolsConfig{
		Search: "rwx main",
		Limit:  1,
		Cursor: "cursor-token",
	})
	require.NoError(t, err)
	require.Equal(t, "/mint/api/concurrency_pools", captured.URL.Path)
	require.Equal(t, "rwx main", captured.URL.Query().Get("search"))
	require.Equal(t, "1", captured.URL.Query().Get("limit"))
	require.Equal(t, "cursor-token", captured.URL.Query().Get("cursor"))
	require.Len(t, result.ConcurrencyPools, 1)
	require.Equal(t, "rwx-cloud/rwx:main", result.ConcurrencyPools[0].ID)
	require.Equal(t, 1, result.ConcurrencyPools[0].LatestConfiguration.Capacity)
	require.Equal(t, "next-page", *result.Pagination.NextCursor)
}

func TestAPIClient_ShowConcurrencyPoolEscapesIDAndDecodesRecentRuns(t *testing.T) {
	var requestURI string
	c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		requestURI = req.URL.RequestURI()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"id":"team/pool:name",
				"status":"idle",
				"queue_size":0,
				"usage_count_last_7_days":1,
				"latest_configuration":null,
				"recent_runs":[{
					"id":"run-1",
					"status":{"result":"succeeded","execution":"finished","waiting_sub_status":null,"aborted_sub_status":null,"finished_sub_status":"not_applicable"},
					"started_at":"2026-10-05T17:00:00Z",
					"completed_at":"2026-10-05T17:01:00Z",
					"completed_runtime_seconds":60,
					"repository_name":null,
					"branch":null,
					"tag":null,
					"commit_sha":null,
					"definition_path":null,
					"trigger":"api",
					"title":null
				}]
			}`)),
		}, nil
	})

	result, err := c.ShowConcurrencyPool("team/pool:name")
	require.NoError(t, err)
	require.Equal(t, "/mint/api/concurrency_pools/team%2Fpool:name", requestURI)
	require.Nil(t, result.LatestConfiguration)
	require.Len(t, result.RecentRuns, 1)
	require.Equal(t, "succeeded", *result.RecentRuns[0].Status.Result)
	require.Nil(t, result.RecentRuns[0].RepositoryName)
}

func TestAPIClient_ListConcurrencyPoolQueueEscapesIDAndPaginates(t *testing.T) {
	var captured *http.Request
	c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		captured = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"leases":[{"id":"lease-1","state":"requested","position":3,"run_id":"run-1","capacity":2,"on_overflow":"queue","requested_at":"2026-10-05T18:00:00Z","acquired_at":null,"revoked_at":null,"released_at":null,"superseded_at":null,"abandoned_at":null,"run":null}],
				"pagination":{"next_cursor":null,"limit":25}
			}`)),
		}, nil
	})

	result, err := c.ListConcurrencyPoolQueue(api.ListConcurrencyPoolQueueConfig{
		PoolID: "team/pool:name",
		Limit:  25,
		Cursor: "queue-cursor",
	})
	require.NoError(t, err)
	require.Equal(t, "/mint/api/concurrency_pools/team%2Fpool:name/queue", captured.URL.EscapedPath())
	require.Equal(t, "25", captured.URL.Query().Get("limit"))
	require.Equal(t, "queue-cursor", captured.URL.Query().Get("cursor"))
	require.Len(t, result.Leases, 1)
	require.Equal(t, 3, result.Leases[0].Position)
	require.Nil(t, result.Pagination.NextCursor)
}

func TestAPIClient_ConcurrencyPoolsSurfacesAPIErrors(t *testing.T) {
	c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     "404 Not Found",
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	_, err := c.ShowConcurrencyPool("missing")
	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrNotFound)
	require.Contains(t, err.Error(), "404 Not Found")
}
