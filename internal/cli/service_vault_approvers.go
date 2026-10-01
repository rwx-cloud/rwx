package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type VaultApproverInfo struct {
	ID    string
	Vault VaultIdentity
	User  UserIdentity
}

type UserIdentity struct {
	ID    string
	Email string
}

type ListVaultApproversConfig struct {
	Vault string
	Json  bool
}

type ListVaultApproversResult struct {
	Approvers []VaultApproverInfo
}

type AddVaultApproverConfig struct {
	Vault string
	Email string
	Json  bool
}

type AddVaultApproverResult = VaultApproverInfo

type RemoveVaultApproverConfig struct {
	Vault      string
	ApproverID string
	Json       bool
	Yes        bool
}

type RemoveVaultApproverResult = VaultApproverInfo

func vaultApproverInfo(approver api.VaultApprover) VaultApproverInfo {
	return VaultApproverInfo{
		ID:    approver.ID,
		Vault: VaultIdentity{ID: approver.Vault.ID, Name: approver.Vault.Name},
		User:  UserIdentity{ID: approver.User.ID, Email: approver.User.Email},
	}
}

func (s Service) ListVaultApprovers(cfg ListVaultApproversConfig) (*ListVaultApproversResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.ListVaultApprovers(api.ListVaultApproversConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list vault approvers")
	}

	approvers := make([]VaultApproverInfo, len(apiResult.Approvers))
	for i, approver := range apiResult.Approvers {
		approvers[i] = vaultApproverInfo(approver)
	}
	result := &ListVaultApproversResult{Approvers: approvers}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(approvers) == 0 {
		fmt.Fprintf(s.Stdout, "No approvers found for vault %q.\n", cfg.Vault)
	} else {
		w := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tUSER ID\tEMAIL")
		for _, approver := range approvers {
			fmt.Fprintf(w, "%s\t%s\t%s\n", approver.ID, approver.User.ID, approver.User.Email)
		}
		if err := w.Flush(); err != nil {
			return nil, errors.Wrap(err, "unable to write vault approver list")
		}
	}
	return result, nil
}

func (s Service) AddVaultApprover(cfg AddVaultApproverConfig) (*AddVaultApproverResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.AddVaultApprover(api.AddVaultApproverConfig{VaultID: vaultID, Email: cfg.Email})
	if err != nil {
		return nil, errors.Wrap(err, "unable to add vault approver")
	}
	result := vaultApproverInfo(apiResult.Approver)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Added %q as an approver for vault %q.\n", result.User.Email, result.Vault.Name)
	}
	return &result, nil
}

func (s Service) RemoveVaultApprover(cfg RemoveVaultApproverConfig) (*RemoveVaultApproverResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.ListVaultApprovers(api.ListVaultApproversConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to find vault approver")
	}
	var approver *api.VaultApprover
	for i := range apiResult.Approvers {
		if apiResult.Approvers[i].ID == cfg.ApproverID {
			approver = &apiResult.Approvers[i]
			break
		}
	}
	if approver == nil {
		return nil, errors.Errorf("vault approver %q was not found", cfg.ApproverID)
	}

	if err := s.confirmDestruction(fmt.Sprintf("Remove %q as an approver for vault %q?", approver.User.Email, approver.Vault.Name), cfg.Yes); err != nil {
		return nil, err
	}
	if _, err := s.APIClient.RemoveVaultApprover(api.RemoveVaultApproverConfig{VaultID: vaultID, ApproverID: approver.ID}); err != nil {
		return nil, errors.Wrap(err, "unable to remove vault approver")
	}
	result := vaultApproverInfo(*approver)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Removed %q as an approver for vault %q.\n", result.User.Email, result.Vault.Name)
	}
	return &result, nil
}
