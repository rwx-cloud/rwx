package cli_test

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_ListCrons(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		s := setupTest(t)
		s.mockAPI.MockListCrons = func() (*api.ListCronsResult, error) { return &api.ListCronsResult{}, nil }
		result, err := s.service.ListCrons(cli.ListCronsConfig{Json: jsonOutput})
		require.NoError(t, err)
		require.Empty(t, result.Crons)
		if jsonOutput {
			require.JSONEq(t, `{"Crons":[]}`, s.mockStdout.String())
		} else {
			require.Equal(t, "No crons found.\n", s.mockStdout.String())
		}
	}
	t.Run("details and PascalCase JSON", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockListCrons = func() (*api.ListCronsResult, error) {
			return &api.ListCronsResult{Crons: []api.Cron{{ID: "cron-1", Key: "nightly", Repository: "org/repo", Branch: "main", RunDefinitionPath: ".rwx/ci.yml", Schedule: "0 0 * * *", TimeZone: "UTC", Status: "paused"}}}, nil
		}
		_, err := s.service.ListCrons(cli.ListCronsConfig{Json: true})
		require.NoError(t, err)
		require.JSONEq(t, `{"Crons":[{"ID":"cron-1","Key":"nightly","Repository":"org/repo","Branch":"main","RunDefinitionPath":".rwx/ci.yml","Schedule":"0 0 * * *","TimeZone":"UTC","Status":"paused","NextInvocationAt":null,"SnoozedUntil":null}]}`, s.mockStdout.String())
		s.mockStdout.Reset()
		_, err = s.service.ListCrons(cli.ListCronsConfig{})
		require.NoError(t, err)
		require.Contains(t, s.mockStdout.String(), "cron-1")
		require.Contains(t, s.mockStdout.String(), ".rwx/ci.yml")
	})
}

func TestService_CronOperations(t *testing.T) {
	for _, action := range []string{"show", "pause", "resume", "snooze"} {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(action, func(t *testing.T) {
				s := setupTest(t)
				mock := func(id string) (*api.CronResult, error) {
					require.Equal(t, "cron-1", id)
					return &api.CronResult{Cron: api.Cron{ID: id, Status: "paused"}}, nil
				}
				cfg := cli.CronConfig{ID: "cron-1", Json: jsonOutput}
				var result *cli.CronResult
				var err error
				switch action {
				case "show":
					s.mockAPI.MockShowCron = mock
					result, err = s.service.ShowCron(cfg)
				case "pause":
					s.mockAPI.MockPauseCron = mock
					result, err = s.service.PauseCron(cfg)
				case "resume":
					s.mockAPI.MockResumeCron = mock
					result, err = s.service.ResumeCron(cfg)
				case "snooze":
					s.mockAPI.MockSnoozeCron = func(id, until string) (*api.CronResult, error) {
						require.Equal(t, "2027-01-02T09:30:59-05:00", until)
						return mock(id)
					}
					result, err = s.service.SnoozeCron(cli.SnoozeCronConfig{CronConfig: cfg, Until: "2027-01-02T09:30:59-05:00"})
				}
				require.NoError(t, err)
				require.Equal(t, "paused", result.Cron.Status)
				if jsonOutput {
					require.Contains(t, s.mockStdout.String(), `"NextInvocationAt":null`)
				} else {
					require.Contains(t, s.mockStdout.String(), "Status: paused\nNext invocation: -\n")
				}
			})
		}
	}
}

func TestService_CronValidationAndErrors(t *testing.T) {
	s := setupTest(t)
	_, err := s.service.ShowCron(cli.CronConfig{})
	require.ErrorContains(t, err, "provide a cron key or use --id")
	for _, until := range []string{"", "tomorrow", "2027-01-02T09:30:00"} {
		_, err = s.service.SnoozeCron(cli.SnoozeCronConfig{CronConfig: cli.CronConfig{ID: "cron-1"}, Until: until})
		require.ErrorContains(t, err, "RFC3339 timestamp")
	}
	s.mockAPI.MockPauseCron = func(string) (*api.CronResult, error) { return nil, errors.New("Cron is archived.") }
	_, err = s.service.PauseCron(cli.CronConfig{ID: "cron-1"})
	require.ErrorContains(t, err, "unable to pause cron: Cron is archived.")
	require.Empty(t, s.mockStdout.String())
}

func TestService_CronKeyResolution(t *testing.T) {
	crons := []api.Cron{
		{ID: "a", Key: "nightly", Repository: "org/app", Branch: "main", RunDefinitionPath: ".rwx/ci.yml"},
		{ID: "b", Key: "nightly", Repository: "org/app", Branch: "main", RunDefinitionPath: ".rwx/deploy.yml"},
		{ID: "c", Key: "nightly", Repository: "org/app", Branch: "release", RunDefinitionPath: ".rwx/ci.yml"},
		{ID: "d", Key: "nightly", Repository: "org/other", Branch: "main", RunDefinitionPath: ".rwx/ci.yml"},
		{ID: "e", Key: "weekly", Repository: "org/app", Branch: "main", RunDefinitionPath: ".rwx/ci.yml"},
	}
	for _, tc := range []struct {
		name      string
		cfg       cli.CronConfig
		wantID    string
		wantError string
	}{
		{name: "unique", cfg: cli.CronConfig{Key: "weekly"}, wantID: "e"},
		{name: "absent", cfg: cli.CronConfig{Key: "missing"}, wantError: "No cron matches"},
		{name: "ambiguous", cfg: cli.CronConfig{Key: "nightly"}, wantError: "More than one cron matches"},
		{name: "repository", cfg: cli.CronConfig{Key: "nightly", Repository: "org/other"}, wantID: "d"},
		{name: "branch", cfg: cli.CronConfig{Key: "nightly", Branch: "release"}, wantID: "c"},
		{name: "file", cfg: cli.CronConfig{Key: "nightly", File: ".rwx/deploy.yml"}, wantID: "b"},
		{name: "all filters", cfg: cli.CronConfig{Key: "nightly", Repository: "org/app", Branch: "main", File: ".rwx/ci.yml"}, wantID: "a"},
		{name: "repository and branch still ambiguous", cfg: cli.CronConfig{Key: "nightly", Repository: "org/app", Branch: "main"}, wantError: "More than one cron matches"},
		{name: "repository and file still ambiguous", cfg: cli.CronConfig{Key: "nightly", Repository: "org/app", File: ".rwx/ci.yml"}, wantError: "More than one cron matches"},
		{name: "branch and file still ambiguous", cfg: cli.CronConfig{Key: "nightly", Branch: "main", File: ".rwx/ci.yml"}, wantError: "More than one cron matches"},
		{name: "conflicting filters", cfg: cli.CronConfig{Key: "nightly", Repository: "org/other", Branch: "release"}, wantError: "No cron matches"},
		{name: "exact repository", cfg: cli.CronConfig{Key: "nightly", Repository: "app"}, wantError: "No cron matches"},
		{name: "exact file", cfg: cli.CronConfig{Key: "nightly", File: "ci.yml"}, wantError: "No cron matches"},
		{name: "explicit ID", cfg: cli.CronConfig{ID: "archived-id"}, wantID: "archived-id"},
		{name: "ID and key", cfg: cli.CronConfig{ID: "a", Key: "nightly"}, wantError: "use --id on its own"},
		{name: "ID and repository", cfg: cli.CronConfig{ID: "a", Repository: "org/app"}, wantError: "use --id on its own"},
		{name: "ID and branch", cfg: cli.CronConfig{ID: "a", Branch: "main"}, wantError: "use --id on its own"},
		{name: "ID and file", cfg: cli.CronConfig{ID: "a", File: ".rwx/ci.yml"}, wantError: "use --id on its own"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setupTest(t)
			listed := false
			s.mockAPI.MockListCrons = func() (*api.ListCronsResult, error) {
				listed = true
				return &api.ListCronsResult{Crons: crons}, nil
			}
			mutated := false
			s.mockAPI.MockPauseCron = func(id string) (*api.CronResult, error) {
				mutated = true
				require.Equal(t, tc.wantID, id)
				return &api.CronResult{Cron: api.Cron{ID: id, Status: "paused"}}, nil
			}
			result, err := s.service.PauseCron(tc.cfg)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, result)
				require.False(t, mutated)
				require.Empty(t, s.mockStdout.String())
				if tc.name == "ambiguous" {
					require.Contains(t, err.Error(), "--repository, --branch, or --file")
					require.Contains(t, err.Error(), "a  repository=org/app  branch=main  file=.rwx/ci.yml")
					require.Contains(t, err.Error(), "d  repository=org/other")
				}
			} else {
				require.NoError(t, err)
				require.True(t, mutated)
				require.Equal(t, tc.wantID, result.Cron.ID)
			}
			if tc.cfg.ID != "" {
				require.False(t, listed, "explicit ID must never list crons")
			}
		})
	}
	t.Run("list failure stops mutation", func(t *testing.T) {
		s := setupTest(t)
		s.mockAPI.MockListCrons = func() (*api.ListCronsResult, error) { return nil, errors.New("offline") }
		_, err := s.service.PauseCron(cli.CronConfig{Key: "nightly"})
		require.ErrorContains(t, err, "unable to find cron: offline")
	})
}

func TestService_CronInteractiveSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		json   bool
		cancel bool
	}{
		{name: "selects second match"},
		{name: "cancel does not mutate", cancel: true},
		{name: "JSON never prompts", json: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setupTestWithTTY(t)
			s.mockAPI.MockListCrons = func() (*api.ListCronsResult, error) {
				return &api.ListCronsResult{Crons: []api.Cron{
					{ID: "first", Key: "nightly", Repository: "org/app", Branch: "main", RunDefinitionPath: ".rwx/ci.yml"},
					{ID: "second", Key: "nightly", Repository: "org/app", Branch: "release", RunDefinitionPath: ".rwx/ci.yml"},
					{ID: "filtered", Key: "nightly", Repository: "org/other", Branch: "main", RunDefinitionPath: ".rwx/ci.yml"},
				}}, nil
			}
			prompted, mutated := false, false
			s.config.ChoicePicker = choicePickerStub{pickOne: func(title string, choices []cli.Choice) (int, error) {
				prompted = true
				require.Equal(t, `Which "nightly" cron?`, title)
				require.Len(t, choices, 2)
				require.Equal(t, "org/app / release / .rwx/ci.yml", choices[1].Label)
				require.Contains(t, choices[1].Description, "second")
				if tc.cancel {
					return 0, errors.New("canceled")
				}
				return 1, nil
			}}
			var err error
			s.service, err = cli.NewService(s.config)
			require.NoError(t, err)
			s.mockAPI.MockPauseCron = func(id string) (*api.CronResult, error) {
				mutated = true
				require.Equal(t, "second", id)
				return &api.CronResult{Cron: api.Cron{ID: id}}, nil
			}
			_, err = s.service.PauseCron(cli.CronConfig{Key: "nightly", Repository: "org/app", Json: tc.json})
			switch {
			case tc.json:
				require.ErrorContains(t, err, "More than one cron matches")
				require.False(t, prompted)
				require.False(t, mutated)
			case tc.cancel:
				require.ErrorContains(t, err, "cron selection canceled")
				require.True(t, prompted)
				require.False(t, mutated)
			default:
				require.NoError(t, err)
				require.True(t, prompted)
				require.True(t, mutated)
			}
		})
	}
}
