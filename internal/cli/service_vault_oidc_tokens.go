package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type CreateVaultOidcTokenConfig struct {
	Vault    string
	Name     string
	Audience string
	Provider string
	Json     bool
}

type VaultIdentity struct {
	ID   string
	Name string
}

type VaultOidcTokenInfo struct {
	ID               string
	Vault            VaultIdentity
	Name             string
	Audience         string
	Subject          string
	Expression       string
	DocumentationURL string
}

type CreateVaultOidcTokenResult = VaultOidcTokenInfo

type ListVaultOidcTokensConfig struct {
	Vault string
	Json  bool
}

type ListVaultOidcTokensResult struct {
	OidcTokens []VaultOidcTokenInfo
}

type VaultOidcTokenConfig struct {
	Vault string
	Token string
	Json  bool
}

type ShowVaultOidcTokenResult = VaultOidcTokenInfo

type UpdateVaultOidcTokenConfig struct {
	Vault    string
	Token    string
	Name     string
	Audience string
	Json     bool
}

type UpdateVaultOidcTokenResult = VaultOidcTokenInfo

type DeleteVaultOidcTokenConfig struct {
	Vault string
	Token string
	Json  bool
	Yes   bool
}

type DeleteVaultOidcTokenResult struct {
	ID    string
	Vault VaultIdentity
	Name  string
}

func vaultOidcTokenInfo(token api.VaultOidcToken) VaultOidcTokenInfo {
	return VaultOidcTokenInfo{
		ID: token.ID,
		Vault: VaultIdentity{
			ID:   token.Vault.ID,
			Name: token.Vault.Name,
		},
		Name:             token.Name,
		Audience:         token.Audience,
		Subject:          token.Subject,
		Expression:       token.Expression,
		DocumentationURL: token.DocumentationURL,
	}
}

func (s Service) resolveVaultID(vault string) (string, error) {
	if vault == "" {
		return "", errors.New("the vault name or ID must be provided")
	}

	result, err := s.APIClient.ListVaults()
	if err != nil {
		return "", errors.Wrap(err, "unable to find vault")
	}

	matches := make([]api.Vault, 0, 1)
	for _, candidate := range result.Vaults {
		if candidate.ID == vault || candidate.Name == vault {
			matches = append(matches, candidate)
		}
	}
	if len(matches) > 1 {
		return "", errors.Errorf("more than one vault matches %q; use its ID", vault)
	}
	if len(matches) == 1 {
		return matches[0].ID, nil
	}
	if _, err := uuid.Parse(vault); err == nil {
		return vault, nil
	}
	return "", errors.Errorf("vault %q was not found", vault)
}

func (s Service) resolveVaultOidcToken(vaultID, token string) (*api.VaultOidcToken, error) {
	if token == "" {
		return nil, errors.New("the OIDC token name or ID must be provided")
	}

	result, err := s.APIClient.ListVaultOidcTokens(api.ListVaultOidcTokensConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to find OIDC token")
	}

	matches := make([]api.VaultOidcToken, 0, 1)
	for _, candidate := range result.OidcTokens {
		if candidate.ID == token || candidate.Name == token {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return nil, errors.Errorf("OIDC token %q was not found", token)
	}
	if len(matches) > 1 {
		return nil, errors.Errorf("more than one OIDC token matches %q; use its ID", token)
	}
	return &matches[0], nil
}

func writeVaultOidcToken(stdout interface{ Write([]byte) (int, error) }, token VaultOidcTokenInfo) {
	fmt.Fprintf(stdout, "ID: %s\n", token.ID)
	fmt.Fprintf(stdout, "Vault: %s (%s)\n", token.Vault.Name, token.Vault.ID)
	fmt.Fprintf(stdout, "Name: %s\n", token.Name)
	fmt.Fprintf(stdout, "Audience: %s\n", token.Audience)
	fmt.Fprintf(stdout, "Subject: %s\n", token.Subject)
	fmt.Fprintf(stdout, "Expression: %s\n", token.Expression)
	fmt.Fprintf(stdout, "Documentation: %s\n", token.DocumentationURL)
}

func (s Service) CreateVaultOidcToken(cfg CreateVaultOidcTokenConfig) (*CreateVaultOidcTokenResult, error) {
	if cfg.Provider == "" {
		if cfg.Name == "" {
			return nil, errors.New("--name is required (or use --provider)")
		}
		if cfg.Audience == "" {
			return nil, errors.New("--audience is required (or use --provider)")
		}
	}

	apiResult, err := s.APIClient.CreateVaultOidcToken(api.CreateVaultOidcTokenConfig{
		VaultName: cfg.Vault,
		Name:      cfg.Name,
		Audience:  cfg.Audience,
		Provider:  cfg.Provider,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create OIDC token")
	}

	result := vaultOidcTokenInfo(*apiResult)
	if result.Name == "" {
		result.Name = cfg.Name
		if result.Name == "" {
			result.Name = cfg.Provider
		}
	}
	if result.Vault.Name == "" {
		result.Vault.Name = cfg.Vault
	}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Created OIDC token %q in vault %q.\n", result.Name, result.Vault.Name)
		fmt.Fprintf(s.Stdout, "\nAudience:   %s\n", result.Audience)
		fmt.Fprintf(s.Stdout, "Subject:    %s\n", result.Subject)
		fmt.Fprintf(s.Stdout, "Expression: %s\n", result.Expression)
		fmt.Fprintf(s.Stdout, "\nFor more information on configuring your identity provider and using your OIDC token, see: %s\n", result.DocumentationURL)
	}

	return &result, nil
}

func (s Service) ListVaultOidcTokens(cfg ListVaultOidcTokensConfig) (*ListVaultOidcTokensResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}

	apiResult, err := s.APIClient.ListVaultOidcTokens(api.ListVaultOidcTokensConfig{VaultID: vaultID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list OIDC tokens")
	}

	tokens := make([]VaultOidcTokenInfo, len(apiResult.OidcTokens))
	for i, token := range apiResult.OidcTokens {
		tokens[i] = vaultOidcTokenInfo(token)
	}
	result := &ListVaultOidcTokensResult{OidcTokens: tokens}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(tokens) == 0 {
		fmt.Fprintf(s.Stdout, "No OIDC tokens found in vault %q.\n", cfg.Vault)
	} else {
		w := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tAUDIENCE\tSUBJECT\tEXPRESSION")
		for _, token := range tokens {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", token.ID, token.Name, token.Audience, token.Subject, token.Expression)
		}
		if err := w.Flush(); err != nil {
			return nil, errors.Wrap(err, "unable to write OIDC token list")
		}
	}

	return result, nil
}

func (s Service) ShowVaultOidcToken(cfg VaultOidcTokenConfig) (*ShowVaultOidcTokenResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	token, err := s.resolveVaultOidcToken(vaultID, cfg.Token)
	if err != nil {
		return nil, err
	}

	apiResult, err := s.APIClient.ShowVaultOidcToken(api.ShowVaultOidcTokenConfig{VaultID: vaultID, TokenID: token.ID})
	if err != nil {
		return nil, errors.Wrap(err, "unable to show OIDC token")
	}
	result := vaultOidcTokenInfo(*apiResult)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		writeVaultOidcToken(s.Stdout, result)
	}
	return &result, nil
}

func (s Service) UpdateVaultOidcToken(cfg UpdateVaultOidcTokenConfig) (*UpdateVaultOidcTokenResult, error) {
	if cfg.Name == "" && cfg.Audience == "" {
		return nil, errors.New("provide --name, --audience, or both")
	}
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	token, err := s.resolveVaultOidcToken(vaultID, cfg.Token)
	if err != nil {
		return nil, err
	}

	apiResult, err := s.APIClient.UpdateVaultOidcToken(api.UpdateVaultOidcTokenConfig{
		VaultID:  vaultID,
		TokenID:  token.ID,
		Name:     cfg.Name,
		Audience: cfg.Audience,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to update OIDC token")
	}
	result := vaultOidcTokenInfo(*apiResult)
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Updated OIDC token %q in vault %q.\n\n", result.Name, result.Vault.Name)
		writeVaultOidcToken(s.Stdout, result)
	}
	return &result, nil
}

func (s Service) DeleteVaultOidcToken(cfg DeleteVaultOidcTokenConfig) (*DeleteVaultOidcTokenResult, error) {
	vaultID, err := s.resolveVaultID(cfg.Vault)
	if err != nil {
		return nil, err
	}
	token, err := s.resolveVaultOidcToken(vaultID, cfg.Token)
	if err != nil {
		return nil, err
	}

	if err := s.confirmDestruction(fmt.Sprintf("Delete OIDC token %q from vault %q?", token.Name, token.Vault.Name), cfg.Yes); err != nil {
		return nil, err
	}
	if _, err := s.APIClient.DeleteVaultOidcToken(api.DeleteVaultOidcTokenConfig{VaultID: vaultID, TokenID: token.ID}); err != nil {
		return nil, errors.Wrap(err, "unable to delete OIDC token")
	}

	result := &DeleteVaultOidcTokenResult{
		ID:    token.ID,
		Vault: VaultIdentity{ID: token.Vault.ID, Name: token.Vault.Name},
		Name:  token.Name,
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Deleted OIDC token %q from vault %q.\n", token.Name, token.Vault.Name)
	}
	return result, nil
}
