package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type GroupRelationshipConfig struct {
	Parent string
	Child  string
	Json   bool
	Yes    bool
}

type GroupRelationshipResult struct {
	ParentGroupID string
	ChildGroupID  string
}

func (s Service) resolveGroupRelationship(cfg GroupRelationshipConfig) (api.GroupRelationship, error) {
	if cfg.Parent == "" || cfg.Child == "" {
		return api.GroupRelationship{}, errors.Wrap(errors.ErrBadRequest, "provide both --parent and --child group names or IDs")
	}
	parent, err := s.ResolveGroup(cfg.Parent)
	if err != nil {
		return api.GroupRelationship{}, errors.Wrap(err, "unable to resolve parent group")
	}
	child, err := s.ResolveGroup(cfg.Child)
	if err != nil {
		return api.GroupRelationship{}, errors.Wrap(err, "unable to resolve child group")
	}
	return api.GroupRelationship{ParentGroupID: parent.ID, ChildGroupID: child.ID}, nil
}

func (s Service) ShowGroupRelationship(cfg GroupRelationshipConfig) (*GroupRelationshipResult, error) {
	pair, err := s.resolveGroupRelationship(cfg)
	if err != nil {
		return nil, err
	}
	result, err := s.APIClient.ShowGroupRelationship(pair)
	if err != nil {
		return nil, errors.Wrap(err, "unable to show group relationship")
	}
	return s.outputGroupRelationship(result.GroupRelationship, cfg.Json, "Child group %s inherits permissions from parent group %s.\n")
}

func (s Service) AddGroupRelationship(cfg GroupRelationshipConfig) (*GroupRelationshipResult, error) {
	pair, err := s.resolveGroupRelationship(cfg)
	if err != nil {
		return nil, err
	}
	result, err := s.APIClient.AddGroupRelationship(pair)
	if err != nil {
		return nil, errors.Wrap(err, "unable to add group relationship")
	}
	return s.outputGroupRelationship(result.GroupRelationship, cfg.Json, "Child group %s now inherits permissions from parent group %s.\n")
}

func (s Service) RemoveGroupRelationship(cfg GroupRelationshipConfig) (*GroupRelationshipResult, error) {
	pair, err := s.resolveGroupRelationship(cfg)
	if err != nil {
		return nil, err
	}
	prompt := fmt.Sprintf("Remove permission inheritance from parent group %s to child group %s? Neither group will be deleted.", pair.ParentGroupID, pair.ChildGroupID)
	if err := s.confirmDestruction(prompt, cfg.Yes); err != nil {
		return nil, err
	}
	if err := s.APIClient.RemoveGroupRelationship(pair); err != nil {
		return nil, errors.Wrap(err, "unable to remove group relationship")
	}
	return s.outputGroupRelationship(pair, cfg.Json, "Removed permission inheritance from parent group %[2]s to child group %[1]s. Neither group was deleted.\n")
}

func (s Service) outputGroupRelationship(pair api.GroupRelationship, jsonOutput bool, message string) (*GroupRelationshipResult, error) {
	result := &GroupRelationshipResult{ParentGroupID: pair.ParentGroupID, ChildGroupID: pair.ChildGroupID}
	if jsonOutput {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, err
		}
	} else {
		fmt.Fprintf(s.Stdout, message, result.ChildGroupID, result.ParentGroupID)
	}
	return result, nil
}
