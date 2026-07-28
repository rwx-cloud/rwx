package cli_test

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_Whoami(t *testing.T) {
	t.Run("when outputting json", func(t *testing.T) {
		t.Run("when the request fails", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return nil, errors.New("uh oh can't figure out who you are")
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: true,
			})

			require.Nil(t, result)
			require.Error(t, err)
			require.Contains(t, err.Error(), "unable to determine details about the access token")
			require.Contains(t, err.Error(), "can't figure out who you are")
		})

		t.Run("when there is an email", func(t *testing.T) {
			s := setupTest(t)

			email := "someone@rwx.com"
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:        "personal_access_token",
					OrganizationSlug: "rwx",
					UserEmail:        &email,
				}, nil
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: true,
			})

			require.NoError(t, err)
			require.Equal(t, "personal_access_token", result.TokenKind)
			require.Equal(t, "rwx", result.OrganizationSlug)
			require.Equal(t, &email, result.UserEmail)
			require.Contains(t, s.mockStdout.String(), `"token_kind": "personal_access_token"`)
			require.Contains(t, s.mockStdout.String(), `"organization_slug": "rwx"`)
			require.Contains(t, s.mockStdout.String(), `"user_email": "someone@rwx.com"`)
		})

		t.Run("when there is not an email", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:        "organization_access_token",
					OrganizationSlug: "rwx",
				}, nil
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: true,
			})

			require.NoError(t, err)
			require.Equal(t, "organization_access_token", result.TokenKind)
			require.Equal(t, "rwx", result.OrganizationSlug)
			require.Nil(t, result.UserEmail)
			require.Contains(t, s.mockStdout.String(), `"token_kind": "organization_access_token"`)
			require.Contains(t, s.mockStdout.String(), `"organization_slug": "rwx"`)
			require.NotContains(t, s.mockStdout.String(), `"user_email"`)
			require.NotContains(t, s.mockStdout.String(), `"service_account_name"`)
		})

		t.Run("when there is a service account name", func(t *testing.T) {
			s := setupTest(t)

			serviceAccountName := "deploy bot"
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:          "organization_access_token",
					OrganizationSlug:   "rwx",
					ServiceAccountName: &serviceAccountName,
				}, nil
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: true,
			})

			require.NoError(t, err)
			require.Equal(t, &serviceAccountName, result.ServiceAccountName)
			require.Contains(t, s.mockStdout.String(), `"token_kind": "organization_access_token"`)
			require.Contains(t, s.mockStdout.String(), `"service_account_name": "deploy bot"`)
		})
	})

	t.Run("when outputting plaintext", func(t *testing.T) {
		t.Run("when the request fails", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return nil, errors.New("uh oh can't figure out who you are")
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: false,
			})

			require.Nil(t, result)
			require.Error(t, err)
			require.Contains(t, err.Error(), "unable to determine details about the access token")
			require.Contains(t, err.Error(), "can't figure out who you are")
		})

		t.Run("when there is an email", func(t *testing.T) {
			s := setupTest(t)

			email := "someone@rwx.com"
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:        "personal_access_token",
					OrganizationSlug: "rwx",
					UserEmail:        &email,
				}, nil
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: false,
			})

			require.NoError(t, err)
			require.Equal(t, "personal_access_token", result.TokenKind)
			require.Equal(t, "rwx", result.OrganizationSlug)
			require.Equal(t, &email, result.UserEmail)
			require.Contains(t, s.mockStdout.String(), "Token Kind: personal access token")
			require.Contains(t, s.mockStdout.String(), "Organization: rwx")
			require.Contains(t, s.mockStdout.String(), "User: someone@rwx.com")
		})

		t.Run("when there is not an email", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:        "organization_access_token",
					OrganizationSlug: "rwx",
				}, nil
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: false,
			})

			require.NoError(t, err)
			require.Equal(t, "organization_access_token", result.TokenKind)
			require.Equal(t, "rwx", result.OrganizationSlug)
			require.Nil(t, result.UserEmail)
			require.Contains(t, s.mockStdout.String(), "Token Kind: organization access token")
			require.Contains(t, s.mockStdout.String(), "Organization: rwx")
			require.NotContains(t, s.mockStdout.String(), "User:")
			require.NotContains(t, s.mockStdout.String(), "Service Account:")
		})

		t.Run("when there is a service account name", func(t *testing.T) {
			s := setupTest(t)

			serviceAccountName := "deploy bot"
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:          "organization_access_token",
					OrganizationSlug:   "rwx",
					ServiceAccountName: &serviceAccountName,
				}, nil
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: false,
			})

			require.NoError(t, err)
			require.Equal(t, &serviceAccountName, result.ServiceAccountName)
			require.Contains(t, s.mockStdout.String(), "Token Kind: service account")
			require.Contains(t, s.mockStdout.String(), "Organization: rwx")
			require.Contains(t, s.mockStdout.String(), "Service Account: deploy bot")
			require.NotContains(t, s.mockStdout.String(), "User:")
		})
	})
}
