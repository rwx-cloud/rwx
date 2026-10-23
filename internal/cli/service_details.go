package cli

import (
	"github.com/rwx-cloud/rwx/internal/api"
)

type GetRunDetailsConfig struct {
	RunID   string
	TaskKey string
	Compact bool
}

func (s Service) GetRunDetails(cfg GetRunDetailsConfig) (map[string]any, error) {
	return s.APIClient.GetRunDetails(api.RunDetailsConfig{
		RunID:   cfg.RunID,
		TaskKey: cfg.TaskKey,
		Compact: cfg.Compact,
	})
}
