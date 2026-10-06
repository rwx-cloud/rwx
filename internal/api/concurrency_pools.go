package api

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type ConcurrencyPoolConfiguration struct {
	Capacity   int    `json:"capacity"`
	OnOverflow string `json:"on_overflow"`
}

type ConcurrencyPoolRunStatus struct {
	Result            *string `json:"result"`
	Execution         string  `json:"execution"`
	WaitingSubStatus  *string `json:"waiting_sub_status"`
	AbortedSubStatus  *string `json:"aborted_sub_status"`
	FinishedSubStatus *string `json:"finished_sub_status"`
}

type ConcurrencyPoolRun struct {
	ID                      string                   `json:"id"`
	Status                  ConcurrencyPoolRunStatus `json:"status"`
	StartedAt               *string                  `json:"started_at"`
	CompletedAt             *string                  `json:"completed_at"`
	CompletedRuntimeSeconds *float64                 `json:"completed_runtime_seconds"`
	RepositoryName          *string                  `json:"repository_name"`
	Branch                  *string                  `json:"branch"`
	Tag                     *string                  `json:"tag"`
	CommitSha               *string                  `json:"commit_sha"`
	DefinitionPath          *string                  `json:"definition_path"`
	Trigger                 string                   `json:"trigger"`
	Title                   *string                  `json:"title"`
}

type ConcurrencyPool struct {
	ID                  string                        `json:"id"`
	Status              string                        `json:"status"`
	PausedAt            *string                       `json:"paused_at"`
	QueueSize           int                           `json:"queue_size"`
	UsageCountLast7Days int                           `json:"usage_count_last_7_days"`
	LastRequestedAt     *string                       `json:"last_requested_at"`
	LatestConfiguration *ConcurrencyPoolConfiguration `json:"latest_configuration"`
	RecentRuns          []ConcurrencyPoolRun          `json:"recent_runs,omitempty"`
}

type ConcurrencyPoolLease struct {
	ID           string              `json:"id"`
	State        string              `json:"state"`
	Position     int                 `json:"position"`
	RunID        string              `json:"run_id"`
	Capacity     int                 `json:"capacity"`
	OnOverflow   string              `json:"on_overflow"`
	RequestedAt  string              `json:"requested_at"`
	AcquiredAt   *string             `json:"acquired_at"`
	RevokedAt    *string             `json:"revoked_at"`
	ReleasedAt   *string             `json:"released_at"`
	SupersededAt *string             `json:"superseded_at"`
	AbandonedAt  *string             `json:"abandoned_at"`
	Run          *ConcurrencyPoolRun `json:"run"`
}

type ConcurrencyPoolPagination struct {
	NextCursor *string `json:"next_cursor"`
	Limit      int     `json:"limit"`
}

type ListConcurrencyPoolsConfig struct {
	Search string
	Limit  int
	Cursor string
}

type ListConcurrencyPoolsResult struct {
	ConcurrencyPools []ConcurrencyPool         `json:"concurrency_pools"`
	Pagination       ConcurrencyPoolPagination `json:"pagination"`
}

type ListConcurrencyPoolQueueConfig struct {
	PoolID string
	Limit  int
	Cursor string
}

type ListConcurrencyPoolQueueResult struct {
	Leases     []ConcurrencyPoolLease    `json:"leases"`
	Pagination ConcurrencyPoolPagination `json:"pagination"`
}

func (c Client) ListConcurrencyPools(cfg ListConcurrencyPoolsConfig) (*ListConcurrencyPoolsResult, error) {
	params := concurrencyPoolPaginationParams(cfg.Limit, cfg.Cursor)
	if cfg.Search != "" {
		params.Set("search", cfg.Search)
	}
	result := &ListConcurrencyPoolsResult{}
	if err := c.concurrencyPoolRequest("/mint/api/concurrency_pools?"+params.Encode(), result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) ShowConcurrencyPool(id string) (*ConcurrencyPool, error) {
	result := &ConcurrencyPool{}
	if err := c.concurrencyPoolRequest("/mint/api/concurrency_pools/"+url.PathEscape(id), result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) ListConcurrencyPoolQueue(cfg ListConcurrencyPoolQueueConfig) (*ListConcurrencyPoolQueueResult, error) {
	params := concurrencyPoolPaginationParams(cfg.Limit, cfg.Cursor)
	endpoint := "/mint/api/concurrency_pools/" + url.PathEscape(cfg.PoolID) + "/queue?" + params.Encode()
	result := &ListConcurrencyPoolQueueResult{}
	if err := c.concurrencyPoolRequest(endpoint, result); err != nil {
		return nil, err
	}
	return result, nil
}

func concurrencyPoolPaginationParams(limit int, cursor string) url.Values {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	return params
}

func (c Client) concurrencyPoolRequest(endpoint string, result any) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()
	if err := decodeResponseJSON(resp, result); err != nil {
		return errors.Wrap(err, "unable to fetch concurrency pool data")
	}
	return nil
}
