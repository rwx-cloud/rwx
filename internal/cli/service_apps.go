package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type AppConfig struct {
	Endpoint string
	Json     bool
}

type AppResult struct {
	Endpoint string
	Disabled bool
}

func (s Service) DisableApp(cfg AppConfig) (*AppResult, error) {
	return s.appOperation(cfg, "disable", s.APIClient.DisableApp)
}

func (s Service) EnableApp(cfg AppConfig) (*AppResult, error) {
	return s.appOperation(cfg, "enable", s.APIClient.EnableApp)
}

func (s Service) appOperation(cfg AppConfig, action string, operation func(string) error) (*AppResult, error) {
	if err := operation(cfg.Endpoint); err != nil {
		return nil, errors.Wrapf(err, "unable to %s app endpoint %q", action, cfg.Endpoint)
	}
	result := &AppResult{Endpoint: cfg.Endpoint, Disabled: action == "disable"}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if result.Disabled {
		fmt.Fprintf(s.Stdout, "Disabled app endpoint %q. Running instances are queued for shutdown.\n", result.Endpoint)
	} else {
		fmt.Fprintf(s.Stdout, "Enabled app endpoint %q. Stopped apps will cold-start on a subsequent visit; permanently expired versions remain expired.\n", result.Endpoint)
	}
	return result, nil
}
