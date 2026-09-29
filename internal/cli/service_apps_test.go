package cli_test

import (
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestService_AppOperations(t *testing.T) {
	for _, action := range []string{"disable", "enable"} {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(action, func(t *testing.T) {
				s := setupTest(t)
				called := false
				mock := func(endpoint string) error {
					called = true
					require.Equal(t, "pr-123", endpoint)
					return nil
				}
				operation := s.service.DisableApp
				s.mockAPI.MockDisableApp = mock
				if action == "enable" {
					operation = s.service.EnableApp
					s.mockAPI.MockEnableApp = mock
				}
				result, err := operation(cli.AppConfig{Endpoint: "pr-123", Json: jsonOutput})
				require.NoError(t, err)
				require.True(t, called)
				require.Equal(t, &cli.AppResult{Endpoint: "pr-123", Disabled: action == "disable"}, result)
				if jsonOutput {
					if action == "disable" {
						require.JSONEq(t, `{"Endpoint":"pr-123","Disabled":true}`, s.mockStdout.String())
					} else {
						require.JSONEq(t, `{"Endpoint":"pr-123","Disabled":false}`, s.mockStdout.String())
					}
				} else if action == "disable" {
					require.Equal(t, "Disabled app endpoint \"pr-123\". Running instances are queued for shutdown.\n", s.mockStdout.String())
				} else {
					require.Equal(t, "Enabled app endpoint \"pr-123\". Stopped apps will cold-start on a subsequent visit; permanently expired versions remain expired.\n", s.mockStdout.String())
				}
			})
		}
		t.Run(action+" error", func(t *testing.T) {
			s := setupTest(t)
			mock := func(string) error { return errors.ErrUnauthenticated }
			operation := s.service.DisableApp
			s.mockAPI.MockDisableApp = mock
			if action == "enable" {
				operation = s.service.EnableApp
				s.mockAPI.MockEnableApp = mock
			}
			result, err := operation(cli.AppConfig{Endpoint: "pr-123", Json: true})
			require.Nil(t, result)
			require.ErrorContains(t, err, "unable to "+action+" app endpoint \"pr-123\"")
			require.ErrorIs(t, err, errors.ErrUnauthenticated)
			require.Empty(t, s.mockStdout.String())
		})
	}
}
