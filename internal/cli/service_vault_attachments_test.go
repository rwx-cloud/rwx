package cli_test

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func testVaultApprover() api.VaultApprover {
	return api.VaultApprover{
		ID:    "identity-1",
		Vault: api.VaultIdentity{ID: "vault-1", Name: "deploys"},
		User:  api.UserIdentity{ID: "user-1", Email: "reviewer@example.com"},
	}
}

func testVaultServiceAccountAttachment() api.VaultServiceAccountAttachment {
	return api.VaultServiceAccountAttachment{
		ID:             "attachment-1",
		Vault:          api.VaultIdentity{ID: "vault-1", Name: "deploys"},
		ServiceAccount: api.ServiceAccountIdentity{ID: "account-1", Name: "deploy-bot"},
		CreatedAt:      "2026-10-01T12:34:56.123456Z",
	}
}

func TestService_VaultApprovers(t *testing.T) {
	t.Run("lists approvers with stable IDs", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultApprovers = func(cfg api.ListVaultApproversConfig) (*api.ListVaultApproversResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			return &api.ListVaultApproversResult{Approvers: []api.VaultApprover{testVaultApprover()}}, nil
		}

		result, err := s.service.ListVaultApprovers(cli.ListVaultApproversConfig{Vault: "deploys"})
		require.NoError(t, err)
		require.Equal(t, "identity-1", result.Approvers[0].ID)
		require.Equal(t, "ID          USER ID  EMAIL\nidentity-1  user-1   reviewer@example.com\n", s.mockStdout.String())
	})

	t.Run("prints the empty state", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultApprovers = func(cfg api.ListVaultApproversConfig) (*api.ListVaultApproversResult, error) {
			return &api.ListVaultApproversResult{}, nil
		}

		_, err := s.service.ListVaultApprovers(cli.ListVaultApproversConfig{Vault: "deploys"})
		require.NoError(t, err)
		require.Equal(t, "No approvers found for vault \"deploys\".\n", s.mockStdout.String())
	})

	t.Run("repeated adds return the same approver", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		calls := 0
		s.mockAPI.MockAddVaultApprover = func(cfg api.AddVaultApproverConfig) (*api.AddVaultApproverResult, error) {
			calls++
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "reviewer@example.com", cfg.Email)
			return &api.AddVaultApproverResult{Approver: testVaultApprover()}, nil
		}

		first, err := s.service.AddVaultApprover(cli.AddVaultApproverConfig{Vault: "deploys", Email: "reviewer@example.com", Json: true})
		require.NoError(t, err)
		second, err := s.service.AddVaultApprover(cli.AddVaultApproverConfig{Vault: "deploys", Email: "reviewer@example.com", Json: true})
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.Equal(t, first.ID, second.ID)
		require.NotContains(t, s.mockStdout.String(), "token")
	})

	t.Run("requires confirmation before removal", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultApprovers = func(cfg api.ListVaultApproversConfig) (*api.ListVaultApproversResult, error) {
			return &api.ListVaultApproversResult{Approvers: []api.VaultApprover{testVaultApprover()}}, nil
		}

		_, err := s.service.RemoveVaultApprover(cli.RemoveVaultApproverConfig{Vault: "deploys", ApproverID: "identity-1"})
		require.ErrorContains(t, err, "use --yes to confirm")
	})

	t.Run("removes by stable approver ID", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultApprovers = func(cfg api.ListVaultApproversConfig) (*api.ListVaultApproversResult, error) {
			return &api.ListVaultApproversResult{Approvers: []api.VaultApprover{testVaultApprover()}}, nil
		}
		s.mockAPI.MockRemoveVaultApprover = func(cfg api.RemoveVaultApproverConfig) (*api.RemoveVaultApproverResult, error) {
			require.Equal(t, "identity-1", cfg.ApproverID)
			return &api.RemoveVaultApproverResult{}, nil
		}

		result, err := s.service.RemoveVaultApprover(cli.RemoveVaultApproverConfig{Vault: "deploys", ApproverID: "identity-1", Yes: true, Json: true})
		require.NoError(t, err)
		require.Equal(t, "identity-1", result.ID)
		require.JSONEq(t, `{"ID":"identity-1","Vault":{"ID":"vault-1","Name":"deploys"},"User":{"ID":"user-1","Email":"reviewer@example.com"}}`, s.mockStdout.String())
	})

	t.Run("surfaces API errors", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockAddVaultApprover = func(cfg api.AddVaultApproverConfig) (*api.AddVaultApproverResult, error) {
			return nil, errors.New("vault:manage permission is required")
		}

		_, err := s.service.AddVaultApprover(cli.AddVaultApproverConfig{Vault: "deploys", Email: "reviewer@example.com"})
		require.ErrorContains(t, err, "vault:manage permission is required")
	})
}

func TestService_VaultServiceAccounts(t *testing.T) {
	t.Run("lists attachment metadata in a table", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultServiceAccountAttachments = func(cfg api.ListVaultServiceAccountAttachmentsConfig) (*api.ListVaultServiceAccountAttachmentsResult, error) {
			return &api.ListVaultServiceAccountAttachmentsResult{ServiceAccountAttachments: []api.VaultServiceAccountAttachment{testVaultServiceAccountAttachment()}}, nil
		}

		_, err := s.service.ListVaultServiceAccounts(cli.ListVaultServiceAccountsConfig{Vault: "deploys"})
		require.NoError(t, err)
		require.Equal(t, "ID            SERVICE ACCOUNT ID  NAME        CREATED AT\nattachment-1  account-1           deploy-bot  2026-10-01T12:34:56.123456Z\n", s.mockStdout.String())
	})

	t.Run("lists attachment metadata as JSON", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultServiceAccountAttachments = func(cfg api.ListVaultServiceAccountAttachmentsConfig) (*api.ListVaultServiceAccountAttachmentsResult, error) {
			require.Equal(t, "vault-1", cfg.VaultID)
			return &api.ListVaultServiceAccountAttachmentsResult{ServiceAccountAttachments: []api.VaultServiceAccountAttachment{testVaultServiceAccountAttachment()}}, nil
		}

		result, err := s.service.ListVaultServiceAccounts(cli.ListVaultServiceAccountsConfig{Vault: "deploys", Json: true})
		require.NoError(t, err)
		require.Equal(t, "attachment-1", result.ServiceAccountAttachments[0].ID)
		require.JSONEq(t, `{"ServiceAccountAttachments":[{"ID":"attachment-1","Vault":{"ID":"vault-1","Name":"deploys"},"ServiceAccount":{"ID":"account-1","Name":"deploy-bot"},"CreatedAt":"2026-10-01T12:34:56.123456Z"}]}`, s.mockStdout.String())
		require.NotContains(t, s.mockStdout.String(), "Token")
		require.NotContains(t, s.mockStdout.String(), "Credential")
	})

	t.Run("prints the empty state", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultServiceAccountAttachments = func(cfg api.ListVaultServiceAccountAttachmentsConfig) (*api.ListVaultServiceAccountAttachmentsResult, error) {
			return &api.ListVaultServiceAccountAttachmentsResult{}, nil
		}

		_, err := s.service.ListVaultServiceAccounts(cli.ListVaultServiceAccountsConfig{Vault: "deploys"})
		require.NoError(t, err)
		require.Equal(t, "No service accounts attached to vault \"deploys\".\n", s.mockStdout.String())
	})

	t.Run("repeated attaches return the same attachment", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		calls := 0
		s.mockAPI.MockAttachVaultServiceAccount = func(cfg api.AttachVaultServiceAccountConfig) (*api.AttachVaultServiceAccountResult, error) {
			calls++
			require.Equal(t, "vault-1", cfg.VaultID)
			require.Equal(t, "deploy-bot", cfg.ServiceAccount)
			return &api.AttachVaultServiceAccountResult{ServiceAccountAttachment: testVaultServiceAccountAttachment()}, nil
		}

		first, err := s.service.AttachVaultServiceAccount(cli.AttachVaultServiceAccountConfig{Vault: "deploys", ServiceAccount: "deploy-bot"})
		require.NoError(t, err)
		second, err := s.service.AttachVaultServiceAccount(cli.AttachVaultServiceAccountConfig{Vault: "deploys", ServiceAccount: "deploy-bot"})
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.Equal(t, first.ID, second.ID)
	})

	t.Run("requires confirmation before detaching", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultServiceAccountAttachments = func(cfg api.ListVaultServiceAccountAttachmentsConfig) (*api.ListVaultServiceAccountAttachmentsResult, error) {
			return &api.ListVaultServiceAccountAttachmentsResult{ServiceAccountAttachments: []api.VaultServiceAccountAttachment{testVaultServiceAccountAttachment()}}, nil
		}

		_, err := s.service.DetachVaultServiceAccount(cli.DetachVaultServiceAccountConfig{Vault: "deploys", AttachmentID: "attachment-1"})
		require.ErrorContains(t, err, "use --yes to confirm")
	})

	t.Run("detaches through the attachment API", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockListVaultServiceAccountAttachments = func(cfg api.ListVaultServiceAccountAttachmentsConfig) (*api.ListVaultServiceAccountAttachmentsResult, error) {
			return &api.ListVaultServiceAccountAttachmentsResult{ServiceAccountAttachments: []api.VaultServiceAccountAttachment{testVaultServiceAccountAttachment()}}, nil
		}
		s.mockAPI.MockDetachVaultServiceAccount = func(cfg api.DetachVaultServiceAccountConfig) (*api.DetachVaultServiceAccountResult, error) {
			require.Equal(t, "attachment-1", cfg.AttachmentID)
			return &api.DetachVaultServiceAccountResult{}, nil
		}

		result, err := s.service.DetachVaultServiceAccount(cli.DetachVaultServiceAccountConfig{Vault: "deploys", AttachmentID: "attachment-1", Yes: true})
		require.NoError(t, err)
		require.Equal(t, "attachment-1", result.ID)
		require.Equal(t, "Detached service account \"deploy-bot\" from vault \"deploys\".\n", s.mockStdout.String())
	})

	t.Run("surfaces API errors", func(t *testing.T) {
		s := setupTest(t)
		configureVaultResolution(s)
		s.mockAPI.MockAttachVaultServiceAccount = func(cfg api.AttachVaultServiceAccountConfig) (*api.AttachVaultServiceAccountResult, error) {
			return nil, errors.New("service_account:manage permission is required")
		}

		_, err := s.service.AttachVaultServiceAccount(cli.AttachVaultServiceAccountConfig{Vault: "deploys", ServiceAccount: "deploy-bot"})
		require.ErrorContains(t, err, "service_account:manage permission is required")
	})
}
