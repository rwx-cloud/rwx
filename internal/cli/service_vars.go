package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/dotenv"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type SetVarsConfig struct {
	Vars  []string
	Vault string
	File  string
	Json  bool
}

func (c SetVarsConfig) Validate() error {
	if c.Vault == "" {
		return errors.New("the vault name must be provided")
	}

	if len(c.Vars) == 0 && c.File == "" {
		return errors.New("the vars to set must be provided")
	}

	return nil
}

type SetVarsResult struct {
	SetVars []string
}

func (s Service) SetVars(cfg SetVarsConfig) (*SetVarsResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	vars := []api.Var{}
	for i := range cfg.Vars {
		key, value, found := strings.Cut(cfg.Vars[i], "=")
		if !found {
			return nil, errors.New(fmt.Sprintf("Invalid var '%s'. Vars must be specified in the form 'KEY=value'.", cfg.Vars[i]))
		}
		vars = append(vars, api.Var{
			Name:  key,
			Value: value,
		})
	}

	if cfg.File != "" {
		fd, err := os.Open(cfg.File)
		if err != nil {
			return nil, errors.Wrapf(err, "error while opening %q", cfg.File)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return nil, errors.Wrapf(err, "error while reading %q", cfg.File)
		}

		dotenvMap := make(map[string]string)
		err = dotenv.ParseBytes(fileContent, dotenvMap)
		if err != nil {
			return nil, errors.Wrapf(err, "error while parsing %q", cfg.File)
		}

		for key, value := range dotenvMap {
			vars = append(vars, api.Var{
				Name:  key,
				Value: value,
			})
		}
	}

	setVars := []string{}
	var setErrors []string
	for _, v := range vars {
		_, err := s.APIClient.SetVar(api.SetVarConfig{
			VaultName: cfg.Vault,
			Var:       v,
		})
		if err != nil {
			setErrors = append(setErrors, fmt.Sprintf("%s: %s", v.Name, err.Error()))
		} else {
			setVars = append(setVars, v.Name)
		}
	}

	result := &SetVarsResult{
		SetVars: setVars,
	}

	if cfg.Json {
		output := struct {
			SetVars []string
		}{
			SetVars: setVars,
		}
		if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(setVars) > 0 {
		fmt.Fprintf(s.Stdout, "Set vars: %s\n", strings.Join(setVars, ", "))
	}

	if len(setErrors) > 0 {
		return result, errors.New(fmt.Sprintf("failed to set some vars:\n%s", strings.Join(setErrors, "\n")))
	}

	return result, nil
}

type ShowVarConfig struct {
	VarName string
	Vault   string
	Json    bool
}

func (c ShowVarConfig) Validate() error {
	if c.VarName == "" {
		return errors.New("the var name must be provided")
	}

	if c.Vault == "" {
		return errors.New("the vault name must be provided")
	}

	return nil
}

type ShowVarResult struct {
	Name  string
	Value string
}

func (s Service) ShowVar(cfg ShowVarConfig) (*ShowVarResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	apiResult, err := s.APIClient.ShowVar(api.ShowVarConfig{
		VarName:   cfg.VarName,
		VaultName: cfg.Vault,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to show var")
	}

	result := &ShowVarResult{
		Name:  apiResult.Name,
		Value: apiResult.Value,
	}

	if cfg.Json {
		output := struct {
			Name  string
			Value string
		}{
			Name:  apiResult.Name,
			Value: apiResult.Value,
		}
		if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintln(s.Stdout, apiResult.Value)
	}

	return result, nil
}

type ListVarsConfig struct {
	Vault string
	Json  bool
}

func (c ListVarsConfig) Validate() error {
	if c.Vault == "" {
		return errors.New("the vault name must be provided")
	}
	return nil
}

type VarInfo struct {
	Name  string
	Value string
}

type ListVarsResult struct {
	Vars []VarInfo
}

func (s Service) ListVars(cfg ListVarsConfig) (*ListVarsResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	apiResult, err := s.APIClient.ListVars(api.ListVarsConfig{VaultName: cfg.Vault})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list vars")
	}

	vars := make([]VarInfo, len(apiResult.Vars))
	for i, variable := range apiResult.Vars {
		vars[i] = VarInfo{Name: variable.Name, Value: variable.Value}
	}
	result := &ListVarsResult{Vars: vars}

	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else if len(vars) == 0 {
		fmt.Fprintf(s.Stdout, "No vars found in vault %q.\n", cfg.Vault)
	} else {
		nameWidth := len("NAME")
		for _, variable := range vars {
			if len(variable.Name) > nameWidth {
				nameWidth = len(variable.Name)
			}
		}
		format := fmt.Sprintf("%%-%ds  %%s\n", nameWidth)
		fmt.Fprintf(s.Stdout, format, "NAME", "VALUE")
		for _, variable := range vars {
			fmt.Fprintf(s.Stdout, format, variable.Name, variable.Value)
		}
	}

	return result, nil
}

type DeleteVarConfig struct {
	VarName string
	Vault   string
	Json    bool
	Yes     bool
}

func (c DeleteVarConfig) Validate() error {
	if c.VarName == "" {
		return errors.New("the var name must be provided")
	}

	if c.Vault == "" {
		return errors.New("the vault name must be provided")
	}

	return nil
}

type DeleteVarResult struct{}

func (s Service) DeleteVar(cfg DeleteVarConfig) (*DeleteVarResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	if err := s.confirmDestruction(
		fmt.Sprintf("Delete var %q from vault %q?", cfg.VarName, cfg.Vault),
		cfg.Yes,
	); err != nil {
		return nil, err
	}

	_, err = s.APIClient.DeleteVar(api.DeleteVarConfig{
		VarName:   cfg.VarName,
		VaultName: cfg.Vault,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to delete var")
	}

	if cfg.Json {
		output := struct {
			Var   string
			Vault string
		}{
			Var:   cfg.VarName,
			Vault: cfg.Vault,
		}
		if err := json.NewEncoder(s.Stdout).Encode(output); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Deleted var %q from vault %q.\n", cfg.VarName, cfg.Vault)
	}

	return &DeleteVarResult{}, nil
}
