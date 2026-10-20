package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type ServiceAccountInfo struct {
	ID          string
	IdentityID  string
	Name        string
	DisplayName string
	Description *string
}

type ServiceAccountSelector struct {
	ServiceAccount string
	DisplayName    string
}

func (cfg ServiceAccountSelector) Validate() error {
	if (cfg.ServiceAccount == "") == (cfg.DisplayName == "") {
		return errors.New("provide a service-account ID or immutable name, or --display-name, but not both")
	}
	return nil
}

func serviceAccountInfo(account api.ServiceAccount) ServiceAccountInfo {
	return ServiceAccountInfo{ID: account.ID, IdentityID: account.IdentityID, Name: account.Name, DisplayName: account.DisplayName, Description: account.Description}
}

func (s Service) ResolveServiceAccount(cfg ServiceAccountSelector) (*ServiceAccountInfo, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(cfg.ServiceAccount); err == nil {
		result, err := s.APIClient.ShowServiceAccount(cfg.ServiceAccount)
		if err != nil {
			return nil, errors.Wrap(err, "unable to show service account")
		}
		account := serviceAccountInfo(result.ServiceAccount)
		return &account, nil
	}
	query := api.ListServiceAccountsConfig{Name: cfg.ServiceAccount, DisplayName: cfg.DisplayName, Limit: 100}
	matches := []api.ServiceAccount{}
	for {
		page, err := s.APIClient.ListServiceAccounts(query)
		if err != nil {
			return nil, errors.Wrap(err, "unable to find service account")
		}
		matches = append(matches, page.ServiceAccounts...)
		query.Cursor = page.Pagination.NextCursor
		if query.Cursor == "" {
			break
		}
	}
	selector := cfg.ServiceAccount
	if cfg.DisplayName != "" {
		selector = cfg.DisplayName
	}
	if len(matches) == 0 {
		return nil, errors.Wrapf(errors.ErrNotFound, "service account %q was not found", selector)
	}
	if len(matches) != 1 {
		return nil, errors.Errorf("more than one service account matches %q; use its ID or immutable name", selector)
	}
	account := serviceAccountInfo(matches[0])
	return &account, nil
}

type ListServiceAccountsConfig struct {
	Limit  int
	Cursor string
	Json   bool
}

type ListServiceAccountsResult struct {
	ServiceAccounts []ServiceAccountInfo
	NextCursor      string
	Limit           int
}

func (s Service) ListServiceAccounts(cfg ListServiceAccountsConfig) (*ListServiceAccountsResult, error) {
	if cfg.Limit == 0 {
		cfg.Limit = 50
	}
	if cfg.Limit < 1 || cfg.Limit > 100 {
		return nil, errors.New("limit must be between 1 and 100")
	}
	page, err := s.APIClient.ListServiceAccounts(api.ListServiceAccountsConfig{Limit: cfg.Limit, Cursor: cfg.Cursor})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list service accounts")
	}
	result := &ListServiceAccountsResult{ServiceAccounts: make([]ServiceAccountInfo, 0, len(page.ServiceAccounts)), NextCursor: page.Pagination.NextCursor, Limit: page.Pagination.Limit}
	for _, account := range page.ServiceAccounts {
		result.ServiceAccounts = append(result.ServiceAccounts, serviceAccountInfo(account))
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		for _, account := range result.ServiceAccounts {
			fmt.Fprintf(s.Stdout, "%s\t%s\t%s\n", account.ID, account.Name, account.DisplayName)
		}
		if len(result.ServiceAccounts) == 0 {
			fmt.Fprintln(s.Stdout, "No service accounts found.")
		}
		if result.NextCursor != "" {
			fmt.Fprintf(s.Stdout, "Next cursor: %s\n", result.NextCursor)
		}
	}
	return result, nil
}

type ShowServiceAccountConfig struct {
	ServiceAccountSelector
	Json bool
}

type ShowServiceAccountResult = ServiceAccountInfo
type CreateServiceAccountResult = ServiceAccountInfo
type UpdateServiceAccountResult = ServiceAccountInfo
type DeactivateServiceAccountResult = ServiceAccountInfo

func (s Service) ShowServiceAccount(cfg ShowServiceAccountConfig) (*ShowServiceAccountResult, error) {
	account, err := s.ResolveServiceAccount(cfg.ServiceAccountSelector)
	if err != nil {
		return nil, err
	}
	if err := s.writeServiceAccount(*account, cfg.Json); err != nil {
		return nil, err
	}
	return account, nil
}

type CreateServiceAccountConfig struct {
	DisplayName string
	Description *string
	Json        bool
}

func (s Service) CreateServiceAccount(cfg CreateServiceAccountConfig) (*CreateServiceAccountResult, error) {
	if strings.TrimSpace(cfg.DisplayName) == "" {
		return nil, errors.New("display name must be provided")
	}
	response, err := s.APIClient.CreateServiceAccount(api.CreateServiceAccountConfig{DisplayName: cfg.DisplayName, Description: cfg.Description})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create service account")
	}
	account := serviceAccountInfo(response.ServiceAccount)
	if !cfg.Json {
		fmt.Fprintln(s.Stdout, "Created service account. No credential was issued.")
	}
	if err := s.writeServiceAccount(account, cfg.Json); err != nil {
		return nil, err
	}
	return &account, nil
}

type UpdateServiceAccountConfig struct {
	ServiceAccountSelector
	DisplayName *string
	Description *string
	Json        bool
}

func (s Service) UpdateServiceAccount(cfg UpdateServiceAccountConfig) (*UpdateServiceAccountResult, error) {
	if cfg.DisplayName == nil && cfg.Description == nil {
		return nil, errors.New("provide --new-display-name or --description to update")
	}
	if cfg.DisplayName != nil && strings.TrimSpace(*cfg.DisplayName) == "" {
		return nil, errors.New("display name must not be empty")
	}
	account, err := s.ResolveServiceAccount(cfg.ServiceAccountSelector)
	if err != nil {
		return nil, err
	}
	response, err := s.APIClient.UpdateServiceAccount(api.UpdateServiceAccountConfig{ID: account.ID, DisplayName: cfg.DisplayName, Description: cfg.Description})
	if err != nil {
		return nil, errors.Wrap(err, "unable to update service account")
	}
	updated := serviceAccountInfo(response.ServiceAccount)
	if err := s.writeServiceAccount(updated, cfg.Json); err != nil {
		return nil, err
	}
	return &updated, nil
}

type DeactivateServiceAccountConfig struct {
	ServiceAccountSelector
	Yes  bool
	Json bool
}

func (s Service) DeactivateServiceAccount(cfg DeactivateServiceAccountConfig) (*DeactivateServiceAccountResult, error) {
	account, err := s.ResolveServiceAccount(cfg.ServiceAccountSelector)
	if err != nil {
		return nil, err
	}
	prompt := fmt.Sprintf("Deactivate service account %q (%s)? This revokes its access tokens and ends its group memberships.", account.Name, account.ID)
	if err := s.confirmDestruction(prompt, cfg.Yes); err != nil {
		return nil, err
	}
	if err := s.APIClient.DeactivateServiceAccount(account.ID); err != nil {
		return nil, errors.Wrap(err, "unable to deactivate service account")
	}
	if !cfg.Json {
		fmt.Fprintln(s.Stdout, "Deactivated service account. Its access tokens were revoked and its group memberships ended.")
	}
	if err := s.writeServiceAccount(*account, cfg.Json); err != nil {
		return nil, err
	}
	return account, nil
}

func (s Service) writeServiceAccount(account ServiceAccountInfo, useJSON bool) error {
	if useJSON {
		return errors.Wrap(json.NewEncoder(s.Stdout).Encode(account), "unable to encode JSON output")
	}
	fmt.Fprintf(s.Stdout, "ID: %s\nIdentity ID: %s\nName: %s\nDisplay name: %s\n", account.ID, account.IdentityID, account.Name, account.DisplayName)
	if account.Description != nil {
		fmt.Fprintf(s.Stdout, "Description: %s\n", *account.Description)
	}
	return nil
}
