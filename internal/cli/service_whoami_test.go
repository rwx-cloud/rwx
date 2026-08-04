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
			tokenDescription := "MacBook"
			tokenURL := "https://cloud.rwx.com/_/personal_access_tokens/123/edit"
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:        "personal_access_token",
					OrganizationSlug: "rwx",
					UserEmail:        &email,
					TokenDescription: &tokenDescription,
					TokenURL:         &tokenURL,
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
			require.Contains(t, s.mockStdout.String(), `"token_description": "MacBook"`)
			require.Contains(t, s.mockStdout.String(), `"token_url": "https://cloud.rwx.com/_/personal_access_tokens/123/edit"`)
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
			require.NotContains(t, s.mockStdout.String(), `"token_description"`)
			require.NotContains(t, s.mockStdout.String(), `"token_url"`)
		})

		t.Run("when there is a service account name", func(t *testing.T) {
			s := setupTest(t)

			serviceAccountName := "deploy bot"
			tokenDescription := "CI release token"
			tokenURL := "https://cloud.rwx.com/org/rwx/manage/service_accounts/123"
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:          "organization_access_token",
					OrganizationSlug:   "rwx",
					ServiceAccountName: &serviceAccountName,
					TokenDescription:   &tokenDescription,
					TokenURL:           &tokenURL,
				}, nil
			}

			result, err := s.service.Whoami(cli.WhoamiConfig{
				Json: true,
			})

			require.NoError(t, err)
			require.Equal(t, &serviceAccountName, result.ServiceAccountName)
			require.Contains(t, s.mockStdout.String(), `"token_kind": "organization_access_token"`)
			require.Contains(t, s.mockStdout.String(), `"service_account_name": "deploy bot"`)
			require.Contains(t, s.mockStdout.String(), `"token_description": "CI release token"`)
			require.Contains(t, s.mockStdout.String(), `"token_url": "https://cloud.rwx.com/org/rwx/manage/service_accounts/123"`)
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
			tokenDescription := "MacBook"
			tokenURL := "https://cloud.rwx.com/_/personal_access_tokens/123/edit"
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:        "personal_access_token",
					OrganizationSlug: "rwx",
					UserEmail:        &email,
					TokenDescription: &tokenDescription,
					TokenURL:         &tokenURL,
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
			require.Contains(t, s.mockStdout.String(), "Token Description: MacBook")
			require.Contains(t, s.mockStdout.String(), "Token URL: https://cloud.rwx.com/_/personal_access_tokens/123/edit")
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
			require.NotContains(t, s.mockStdout.String(), "Token Description:")
			require.NotContains(t, s.mockStdout.String(), "Token URL:")
		})

		t.Run("when there is a service account name", func(t *testing.T) {
			s := setupTest(t)

			serviceAccountName := "deploy bot"
			tokenDescription := "CI release token"
			tokenURL := "https://cloud.rwx.com/org/rwx/manage/service_accounts/123"
			s.mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
				return &api.WhoamiResult{
					TokenKind:          "organization_access_token",
					OrganizationSlug:   "rwx",
					ServiceAccountName: &serviceAccountName,
					TokenDescription:   &tokenDescription,
					TokenURL:           &tokenURL,
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
			require.Contains(t, s.mockStdout.String(), "Token Description: CI release token")
			require.Contains(t, s.mockStdout.String(), "Token URL: https://cloud.rwx.com/org/rwx/manage/service_accounts/123")
			require.NotContains(t, s.mockStdout.String(), "User:")
		})
	})
}
