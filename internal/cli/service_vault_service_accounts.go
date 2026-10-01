package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ServiceAccountIdentity struct {
	ID   string
	Name string
}

type VaultServiceAccountAttachmentInfo struct {
	ID             string
	Vault          VaultIdentity
	ServiceAccount ServiceAccountIdentity
	Expression     string
	CreatedAt      string
}

type ListVaultServiceAccountsConfig struct {
	Vault string
	Json  bool
}

type ListVaultServiceAccountsResult struct {
	ServiceAccountAttachments []VaultServiceAccountAttachmentInfo
}

type AttachVaultServiceAccountConfig struct {
	Vault          string
	ServiceAccount string
	Json           bool
}

type AttachVaultServiceAccountResult = VaultServiceAccountAttachmentInfo

type DetachVaultServiceAccountConfig struct {
	Vault        string
	AttachmentID string
	Json         bool
	Yes          bool
}

type DetachVaultServiceAccountResult = VaultServiceAccountAttachmentInfo

func vaultServiceAccountAttachmentInfo(attachment api.VaultServiceAccountAttachment) VaultServiceAccountAttachmentInfo {
	return VaultServiceAccountAttachmentInfo{
		ID:             attachment.ID,
		Vault:          VaultIdentity{ID: attachment.Vault.ID, Name: attachment.Vault.Name},
		ServiceAccount: ServiceAccountIdentity{ID: attachment.ServiceAccount.ID, Name: attachment.ServiceAccount.Name},
		Expression:     attachment.Expression,
		CreatedAt:      attachment.CreatedAt,
	}
}

func (s Service) ListVaultServiceAccounts(cfg ListVaultServiceAccountsConfig) (*ListVaultServiceAccountsResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.ListVaultServiceAccountAttachments(api.ListVaultServiceAccountAttachmentsConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list attached service accounts")
	}

	attachments := make([]VaultServiceAccountAttachmentInfo, len(apiResult.ServiceAccountAttachments))
	for i, attachment := range apiResult.ServiceAccountAttachments {
		attachments[i] = vaultServiceAccountAttachmentInfo(attachment)
	}
	result := &ListVaultServiceAccountsResult{ServiceAccountAttachments: attachments}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(attachments) == 0 {
		fmt.Fprintf(s.Stdout, "No service accounts attached to vault %q.\n", cfg.Vault)
	} else {
		w := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSERVICE ACCOUNT ID\tNAME\tEXPRESSION\tCREATED AT")
		for _, attachment := range attachments {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", attachment.ID, attachment.ServiceAccount.ID, attachment.ServiceAccount.Name, attachment.Expression, attachment.CreatedAt)
		}
		if err := w.Flush(); err != nil {
			return nil, errors.Wrap(err, "unable to write attached service account list")
		}
	}
	return result, nil
}

func (s Service) AttachVaultServiceAccount(cfg AttachVaultServiceAccountConfig) (*AttachVaultServiceAccountResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.AttachVaultServiceAccount(api.AttachVaultServiceAccountConfig{
		VaultID: vaultID, ServiceAccount: cfg.ServiceAccount,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to attach service account")
	}
	result := vaultServiceAccountAttachmentInfo(apiResult.ServiceAccountAttachment)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Attached service account %q to vault %q.\n", result.ServiceAccount.Name, result.Vault.Name)
		fmt.Fprintf(s.Stdout, "Expression: %s\n", result.Expression)
	}
	return &result, nil
}

func (s Service) DetachVaultServiceAccount(cfg DetachVaultServiceAccountConfig) (*DetachVaultServiceAccountResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	apiResult, err := s.APIClient.ListVaultServiceAccountAttachments(api.ListVaultServiceAccountAttachmentsConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to find service account attachment")
	}
	var attachment *api.VaultServiceAccountAttachment
	for i := range apiResult.ServiceAccountAttachments {
		if apiResult.ServiceAccountAttachments[i].ID == cfg.AttachmentID {
			attachment = &apiResult.ServiceAccountAttachments[i]
			break
		}
	}
	if attachment == nil {
		return nil, errors.Errorf("service account attachment %q was not found", cfg.AttachmentID)
	}

	if err := s.confirmDestruction(fmt.Sprintf("Detach service account %q from vault %q?", attachment.ServiceAccount.Name, attachment.Vault.Name), cfg.Yes); err != nil {
		return nil, err
	}
	if _, err := s.APIClient.DetachVaultServiceAccount(api.DetachVaultServiceAccountConfig{VaultID: vaultID, AttachmentID: attachment.ID}); err != nil {
		return nil, errors.Wrap(err, "unable to detach service account")
	}
	result := vaultServiceAccountAttachmentInfo(*attachment)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Detached service account %q from vault %q.\n", result.ServiceAccount.Name, result.Vault.Name)
	}
	return &result, nil
}
