package cli

import (
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type GetRunStatusConfig struct {
	RunID          string
	BranchName     string
	RepositoryName string
	Wait           bool
	FailFast       bool
	Json           bool
}

type GetRunStatusResult struct {
	RunID        string
	RunURL       string
	Commit       string
	ResultStatus string
	Completed    bool
}

func (s Service) GetRunStatus(cfg GetRunStatusConfig) (*GetRunStatusResult, error) {
	waitStart := time.Now()
	var stopSpinner func()
	if cfg.Wait && !cfg.Json {
		stopSpinner = Spin("Waiting for run to complete...", s.StdoutIsTTY, s.Stdout)
	}

	for {
		statusResult, err := s.APIClient.RunStatus(api.RunStatusConfig{
			RunID:          cfg.RunID,
			BranchName:     cfg.BranchName,
			RepositoryName: cfg.RepositoryName,
			FailFast:       cfg.FailFast,
		})
		if err != nil {
			if stopSpinner != nil {
				stopSpinner()
			}
			return nil, errors.Wrap(err, "unable to get run status")
		}

		status := ""
		if statusResult.Status != nil {
			status = statusResult.Status.Result
		}

		if !cfg.Wait || statusResult.Polling.Completed {
			if stopSpinner != nil {
				stopSpinner()
			}
			runID := cfg.RunID
			if statusResult.RunID != "" {
				runID = statusResult.RunID
			}
			commit := ""
			if statusResult.Commit != nil {
				commit = *statusResult.Commit
			}

			if statusResult.Polling.Completed {
				s.recordTelemetry("run.complete", map[string]any{
					"result_status":    status,
					"wait_duration_ms": time.Since(waitStart).Milliseconds(),
					"wait":             cfg.Wait,
				})
			}

			return &GetRunStatusResult{
				RunID:        runID,
				RunURL:       statusResult.RunURL,
				Commit:       commit,
				ResultStatus: status,
				Completed:    statusResult.Polling.Completed,
			}, nil
		}

		if statusResult.Polling.BackoffMs == nil {
			if stopSpinner != nil {
				stopSpinner()
			}
			return nil, errors.New("unable to wait for run")
		}
		time.Sleep(time.Duration(*statusResult.Polling.BackoffMs) * time.Millisecond)
	}
}
