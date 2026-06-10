package cli_test

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestService_GetRunDetails(t *testing.T) {
	t.Run("passes the run ID and task key through to the API", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockGetRunDetails = func(cfg api.RunDetailsConfig) (map[string]any, error) {
			require.Equal(t, "run-123", cfg.RunID)
			require.Equal(t, "ci.rspec", cfg.TaskKey)
			return map[string]any{"id": "run-123"}, nil
		}

		details, err := setup.service.GetRunDetails(cli.GetRunDetailsConfig{
			RunID:   "run-123",
			TaskKey: "ci.rspec",
		})

		require.NoError(t, err)
		require.Equal(t, map[string]any{"id": "run-123"}, details)
	})

	t.Run("returns error when API fails", func(t *testing.T) {
		setup := setupTest(t)

		setup.mockAPI.MockGetRunDetails = func(cfg api.RunDetailsConfig) (map[string]any, error) {
			return nil, errors.New("422 Unprocessable Entity")
		}

		details, err := setup.service.GetRunDetails(cli.GetRunDetailsConfig{RunID: "run-123"})

		require.Nil(t, details)
		require.Error(t, err)
	})
}
