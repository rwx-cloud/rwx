package cli

import (
	"fmt"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

const deferredRunDefaultBackoffMs = 2000

// awaitDeferredRun handles the 202 deferred response returned when an ephemeral
// org's task servers are cold-starting. It prints the placeholder URL up front
// (so a user who Ctrl+Cs can still open the page), then blocks polling the
// deferred-run status endpoint until the run is created or expired. On success
// it returns a result shaped like a normal created run so the caller's flow
// (print, --open, --wait, --debug) is unchanged.
func (s Service) awaitDeferredRun(deferred *api.InitiateRunResult, cfg InitiateRunConfig) (*api.InitiateRunResult, error) {
	if !cfg.Json {
		fmt.Fprintf(s.Stdout, "Your run will begin shortly.\nTrack it here: %s\n", deferred.PlaceholderURL)
	}

	var stopSpinner func()
	if !cfg.Json {
		stopSpinner = Spin("Waiting for run to start...", s.StdoutIsTTY, s.Stdout)
	}

	waitStart := time.Now()
	for {
		status, err := s.APIClient.DeferredRunStatus(deferred.PollingURL)
		if err != nil {
			if stopSpinner != nil {
				stopSpinner()
			}
			return nil, errors.Wrap(err, "unable to get deferred run status")
		}

		if status.Polling.Completed {
			if stopSpinner != nil {
				stopSpinner()
			}

			s.recordTelemetry("run.deferred", map[string]any{
				"state":            status.State,
				"wait_duration_ms": time.Since(waitStart).Milliseconds(),
			})

			switch status.State {
			case api.DeferredRunStateCreated:
				return &api.InitiateRunResult{
					RunID:   status.RunID,
					RunURL:  status.RunURL,
					Message: fmt.Sprintf("Run is watchable at %s\n", status.RunURL),
				}, nil
			case api.DeferredRunStateExpired:
				msg := status.FailureReason
				if msg == "" {
					msg = "The run could not be started before the request expired. Please try again."
				}
				return nil, errors.New(msg)
			default:
				return nil, errors.Errorf("deferred run finished in an unexpected state: %q", status.State)
			}
		}

		backoffMs := deferredRunDefaultBackoffMs
		if status.Polling.BackoffMs != nil {
			backoffMs = *status.Polling.BackoffMs
		}
		time.Sleep(time.Duration(backoffMs) * time.Millisecond)
	}
}
