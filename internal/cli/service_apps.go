package cli

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ShareAppConfig struct {
	Endpoint    string
	Description string
	ExpiresIn   string
	Json        bool
}

type ShareAppResult struct {
	URL         string
	ExpiresAt   string
	Description string
}

func (s Service) ShareApp(cfg ShareAppConfig) (*ShareAppResult, error) {
	duration, err := parseAppShareDuration(cfg.ExpiresIn)
	if err != nil {
		return nil, errors.Wrap(err, "--expires-in must be a duration (e.g. 1h, 1d, 7d)")
	}
	if duration <= 0 {
		return nil, errors.New("--expires-in must be greater than zero")
	}
	apiResult, err := s.APIClient.ShareApp(api.ShareAppConfig{
		Endpoint:    cfg.Endpoint,
		Description: cfg.Description,
		ExpiresAt:   time.Now().Add(duration).UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to share app endpoint %q", cfg.Endpoint)
	}
	result := &ShareAppResult{
		URL: apiResult.URL, ExpiresAt: apiResult.ExpiresAt, Description: apiResult.Description,
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintln(s.Stdout, result.URL)
	}
	return result, nil
}

var appShareDurationPart = regexp.MustCompile(`^([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(ns|us|µs|μs|ms|s|m|h|d)`)

func parseAppShareDuration(value string) (time.Duration, error) {
	if !strings.Contains(value, "d") {
		return time.ParseDuration(value)
	}
	var normalized strings.Builder
	rest := value
	if strings.HasPrefix(rest, "+") || strings.HasPrefix(rest, "-") {
		normalized.WriteByte(rest[0])
		rest = rest[1:]
	}
	for rest != "" {
		part := appShareDurationPart.FindStringSubmatch(rest)
		if part == nil {
			return 0, errors.Errorf("invalid duration %q", value)
		}
		if part[2] == "d" {
			// Decimal arithmetic preserves precision; ParseDuration checks overflow after conversion.
			hours, _ := new(big.Rat).SetString(part[1])
			hours.Mul(hours, big.NewRat(24, 1))
			normalized.WriteString(hours.FloatString(len(part[1])))
			normalized.WriteByte('h')
		} else {
			normalized.WriteString(part[0])
		}
		rest = rest[len(part[0]):]
	}
	return time.ParseDuration(normalized.String())
}

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
