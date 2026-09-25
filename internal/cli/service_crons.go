package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type CronInfo struct {
	ID                string
	Key               string
	Repository        string
	Branch            string
	RunDefinitionPath string
	Schedule          string
	TimeZone          string
	Status            string
	NextInvocationAt  *string
	SnoozedUntil      *string
}

type ListCronsConfig struct {
	Json bool
}

type ListCronsResult struct {
	Crons []CronInfo
}

type CronConfig struct {
	ID         string
	Key        string
	Repository string
	Branch     string
	File       string
	Json       bool
}

type SnoozeCronConfig struct {
	CronConfig
	Until string
}

type CronResult struct {
	Cron CronInfo
}

func cronInfo(cron api.Cron) CronInfo {
	return CronInfo{
		ID:                cron.ID,
		Key:               cron.Key,
		Repository:        cron.Repository,
		Branch:            cron.Branch,
		RunDefinitionPath: cron.RunDefinitionPath,
		Schedule:          cron.Schedule,
		TimeZone:          cron.TimeZone,
		Status:            cron.Status,
		NextInvocationAt:  cron.NextInvocationAt,
		SnoozedUntil:      cron.SnoozedUntil,
	}
}

func (cfg CronConfig) Validate() error {
	if cfg.ID == "" && cfg.Key == "" {
		return errors.New("provide a cron key or use --id")
	}
	if cfg.ID != "" && (cfg.Key != "" || cfg.Repository != "" || cfg.Branch != "" || cfg.File != "") {
		return errors.New("use --id on its own, without a cron key, --repository, --branch, or --file")
	}
	return nil
}

func (s Service) resolveCron(cfg CronConfig) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", errors.Wrap(err, "validation failed")
	}
	if cfg.ID != "" {
		return cfg.ID, nil
	}
	result, err := s.APIClient.ListCrons()
	if err != nil {
		return "", errors.Wrap(err, "unable to find cron")
	}
	var matches []api.Cron
	for _, cron := range result.Crons {
		if cron.Key == cfg.Key &&
			(cfg.Repository == "" || cron.Repository == cfg.Repository) &&
			(cfg.Branch == "" || cron.Branch == cfg.Branch) &&
			(cfg.File == "" || cron.RunDefinitionPath == cfg.File) {
			matches = append(matches, cron)
		}
	}
	if len(matches) == 0 {
		return "", errors.New(fmt.Sprintf("No cron matches %q with these filters. Use 'rwx crons list' to see available crons, or --id to select an archived cron.", cfg.Key))
	}
	if len(matches) > 1 {
		if s.StdoutIsTTY && !cfg.Json {
			choices := make([]Choice, len(matches))
			for i, cron := range matches {
				choices[i] = Choice{
					Label:       fmt.Sprintf("%s / %s / %s", cron.Repository, cron.Branch, cron.RunDefinitionPath),
					Description: fmt.Sprintf("%s · %s · %s (%s)", cron.ID, cron.Status, cron.Schedule, cron.TimeZone),
				}
			}
			selected, err := s.ChoicePicker.PickOne(fmt.Sprintf("Which %q cron?", cfg.Key), choices)
			if err != nil {
				return "", errors.Wrap(err, "cron selection canceled")
			}
			return matches[selected].ID, nil
		}
		var message strings.Builder
		fmt.Fprintf(&message, "More than one cron matches %q. Narrow it down with --repository, --branch, or --file, or use --id:\n", cfg.Key)
		for _, cron := range matches {
			fmt.Fprintf(&message, "  %s  repository=%s  branch=%s  file=%s\n", cron.ID, cron.Repository, cron.Branch, cron.RunDefinitionPath)
		}
		return "", errors.New(strings.TrimSpace(message.String()))
	}
	return matches[0].ID, nil
}

func (s Service) ListCrons(cfg ListCronsConfig) (*ListCronsResult, error) {
	response, err := s.APIClient.ListCrons()
	if err != nil {
		return nil, errors.Wrap(err, "unable to list crons")
	}
	result := &ListCronsResult{Crons: make([]CronInfo, len(response.Crons))}
	for i, cron := range response.Crons {
		result.Crons[i] = cronInfo(cron)
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(result.Crons) == 0 {
		fmt.Fprintln(s.Stdout, "No crons found.")
	} else {
		w := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tKEY\tREPOSITORY\tBRANCH\tRUN DEFINITION\tSCHEDULE\tTIME ZONE\tSTATUS\tNEXT INVOCATION\tSNOOZED UNTIL")
		for _, cron := range result.Crons {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", cron.ID, cron.Key, cron.Repository, cron.Branch, cron.RunDefinitionPath, cron.Schedule, cron.TimeZone, cron.Status, cronTimestamp(cron.NextInvocationAt), cronTimestamp(cron.SnoozedUntil))
		}
		if err := w.Flush(); err != nil {
			return nil, errors.Wrap(err, "unable to write cron list")
		}
	}
	return result, nil
}

func (s Service) ShowCron(cfg CronConfig) (*CronResult, error) {
	return s.cronOperation(cfg, "show", s.APIClient.ShowCron)
}

func (s Service) PauseCron(cfg CronConfig) (*CronResult, error) {
	return s.cronOperation(cfg, "pause", s.APIClient.PauseCron)
}

func (s Service) ResumeCron(cfg CronConfig) (*CronResult, error) {
	return s.cronOperation(cfg, "resume", s.APIClient.ResumeCron)
}

func (s Service) SnoozeCron(cfg SnoozeCronConfig) (*CronResult, error) {
	if _, err := time.Parse(time.RFC3339, cfg.Until); err != nil {
		return nil, errors.Wrap(err, "until must be an RFC3339 timestamp with a time zone (e.g. 2027-01-02T09:00:00-05:00)")
	}
	return s.cronOperation(cfg.CronConfig, "snooze", func(id string) (*api.CronResult, error) {
		return s.APIClient.SnoozeCron(id, cfg.Until)
	})
}

func (s Service) cronOperation(cfg CronConfig, action string, operation func(string) (*api.CronResult, error)) (*CronResult, error) {
	id, err := s.resolveCron(cfg)
	if err != nil {
		return nil, err
	}
	response, err := operation(id)
	if err != nil {
		return nil, errors.Wrap(err, "unable to "+action+" cron")
	}
	result := &CronResult{Cron: cronInfo(response.Cron)}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		cron := result.Cron
		fmt.Fprintf(s.Stdout, "ID: %s\nKey: %s\nRepository: %s\nBranch: %s\nRun definition: %s\nSchedule: %s\nTime zone: %s\nStatus: %s\nNext invocation: %s\nSnoozed until: %s\n", cron.ID, cron.Key, cron.Repository, cron.Branch, cron.RunDefinitionPath, cron.Schedule, cron.TimeZone, cron.Status, cronTimestamp(cron.NextInvocationAt), cronTimestamp(cron.SnoozedUntil))
	}
	return result, nil
}

func cronTimestamp(value *string) string {
	if value == nil {
		return "-"
	}
	return *value
}
