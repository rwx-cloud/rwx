package cli

import (
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
	})
}
