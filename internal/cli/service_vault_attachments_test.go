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
