package cli

import (
	"regexp"
	"strconv"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type GetRunDetailsConfig struct {
	RunID   string
	TaskKey string
}

func (s Service) GetRunDetails(cfg GetRunDetailsConfig) (map[string]any, error) {
	return s.APIClient.GetRunDetails(api.RunDetailsConfig{
		RunID:   cfg.RunID,
		TaskKey: cfg.TaskKey,
	})
}

func (s Service) ResolveTaskAttempt(id, taskKey, attempt string) (string, error) {
	if !regexp.MustCompile(`^[1-9][0-9]{0,8}$`).MatchString(attempt) {
		return "", errors.New("--attempt must be a positive integer with at most 9 digits")
	}
	if id == "" {
		return "", errors.New("--attempt requires a task ID or --task")
	}

	summary, err := s.APIClient.GetRunDetails(api.RunDetailsConfig{
		RunID: id, TaskKey: taskKey, View: "summary", AttemptNumber: attempt,
	})
	if err != nil {
		return "", errors.Wrap(err, "unable to resolve task attempt")
	}
	taskID, _ := summary["id"].(string)
	attemptNumber, _ := summary["attempt_number"].(float64)
	want, _ := strconv.Atoi(attempt)
	if taskID == "" || attemptNumber != float64(want) {
		return "", errors.New("requested task attempt was not returned by the API")
	}
	return taskID, nil
}
