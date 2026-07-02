package cli

import (
	"io"

	"github.com/rwx-cloud/rwx/internal/api"
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
