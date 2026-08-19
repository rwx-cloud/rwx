package cli

import (
	"io"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ListRunsConfig struct {
	RepositoryNames   []string
	Branches          []string
	CommitShas        []string
	DefinitionPaths   []string
	ResultStatuses    []string
	ExecutionStatuses []string
	MyRuns            bool
	Limit             int
	Cursor            string

	// RetryProgress, when non-nil, receives a notice each time a rate-limited
	// or transient request is retried. The command routes it to stderr under
	// --json so structured stdout stays clean.
	RetryProgress io.Writer
}

func (s Service) ListRuns(cfg ListRunsConfig) (*api.ListRunsResult, error) {
	return s.APIClient.ListRuns(api.ListRunsConfig{
		RepositoryNames:   cfg.RepositoryNames,
		Branches:          cfg.Branches,
		CommitShas:        cfg.CommitShas,
		DefinitionPaths:   cfg.DefinitionPaths,
		ResultStatuses:    cfg.ResultStatuses,
		ExecutionStatuses: cfg.ExecutionStatuses,
		MyRuns:            cfg.MyRuns,
		Limit:             cfg.Limit,
		Cursor:            cfg.Cursor,
		RetryProgress:     cfg.RetryProgress,
	})
}

type CancelRunConfig struct {
	RunID string
}

type CancelRunResult struct {
	RunID string
}

// CancelRun cancels a run the caller identifies by ID. It sends no scoped token,
// so the API client authenticates with the user's access token.
func (s Service) CancelRun(cfg CancelRunConfig) (*CancelRunResult, error) {
	if cfg.RunID == "" {
		return nil, errors.New("a run ID is required")
	}

	if err := s.APIClient.CancelRun(cfg.RunID, ""); err != nil {
		return nil, err
	}

	return &CancelRunResult{RunID: cfg.RunID}, nil
}
