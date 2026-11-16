package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type GroupInfo struct {
	ID          string
	Name        string
	Description *string
}

type GroupPagination struct {
	NextCursor *string
	Limit      int
}

type ListGroupsConfig struct {
	Limit  int
	Cursor string
	Json   bool
}

type ListGroupsResult struct {
	Groups     []GroupInfo
	Pagination GroupPagination
}

type ShowGroupConfig struct {
	Group string
	Json  bool
}

type CreateGroupConfig struct {
	Name        string
	Description *string
	Json        bool
}

type UpdateGroupConfig struct {
	Group       string
	Name        *string
	Description *string
	Json        bool
}

type DeleteGroupConfig struct {
	Group string
	Yes   bool
	Json  bool
}

type ShowGroupResult = GroupInfo
type CreateGroupResult = GroupInfo
type UpdateGroupResult = GroupInfo
type DeleteGroupResult = GroupInfo

func groupInfo(group api.Group) GroupInfo {
	return GroupInfo{ID: group.ID, Name: group.Name, Description: group.Description}
}

func (s Service) ResolveGroup(nameOrID string) (*GroupInfo, error) {
	if nameOrID == "" {
		return nil, errors.Wrap(errors.ErrBadRequest, "provide a group name or ID")
	}
	var match *GroupInfo
	cfg := api.ListGroupsConfig{Limit: 100}
	for {
		page, err := s.APIClient.ListGroups(cfg)
		if err != nil {
			return nil, errors.Wrap(err, "unable to resolve group")
		}
		for _, group := range page.Groups {
			if group.ID != nameOrID && group.Name != nameOrID {
				continue
			}
			if match != nil && match.ID != group.ID {
				return nil, errors.Wrapf(errors.ErrBadRequest, "group %q is ambiguous; use a unique group ID", nameOrID)
			}
			info := groupInfo(group)
			match = &info
		}
		if page.Pagination.NextCursor == nil || *page.Pagination.NextCursor == "" {
			break
		}
		cfg.Cursor = *page.Pagination.NextCursor
	}
	if match == nil {
		return nil, errors.Wrapf(errors.ErrNotFound, "group %q was not found", nameOrID)
	}
	return match, nil
}

func (s Service) ListGroups(cfg ListGroupsConfig) (*ListGroupsResult, error) {
	if cfg.Limit < 0 || cfg.Limit > 100 {
		return nil, errors.Wrap(errors.ErrBadRequest, "limit must be between 1 and 100")
	}
	page, err := s.APIClient.ListGroups(api.ListGroupsConfig{Limit: cfg.Limit, Cursor: cfg.Cursor})
	if err != nil {
		return nil, errors.Wrap(err, "unable to list groups")
	}
	groups := make([]GroupInfo, len(page.Groups))
	for i, group := range page.Groups {
		groups[i] = groupInfo(group)
	}
	result := &ListGroupsResult{Groups: groups, Pagination: GroupPagination{NextCursor: page.Pagination.NextCursor, Limit: page.Pagination.Limit}}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, err
		}
	} else {
		s.writeGroups(groups)
		if result.Pagination.NextCursor != nil && *result.Pagination.NextCursor != "" {
			fmt.Fprintf(s.Stderr, "\nMore results available. Fetch the next page with --cursor %s\n", *result.Pagination.NextCursor)
		}
	}
	return result, nil
}

func (s Service) ShowGroup(cfg ShowGroupConfig) (*ShowGroupResult, error) {
	group, err := s.ResolveGroup(cfg.Group)
	if err != nil {
		return nil, err
	}
	result, err := s.APIClient.ShowGroup(group.ID)
	if err != nil {
		return nil, errors.Wrap(err, "unable to show group")
	}
	return s.outputGroup(result.Group, cfg.Json)
}

func (s Service) CreateGroup(cfg CreateGroupConfig) (*CreateGroupResult, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, errors.Wrap(errors.ErrBadRequest, "the group name must not be empty")
	}
	result, err := s.APIClient.CreateGroup(api.CreateGroupConfig{Name: cfg.Name, Description: cfg.Description})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create group")
	}
	return s.outputGroup(result.Group, cfg.Json)
}

func (s Service) UpdateGroup(cfg UpdateGroupConfig) (*UpdateGroupResult, error) {
	if cfg.Name == nil && cfg.Description == nil {
		return nil, errors.Wrap(errors.ErrBadRequest, "provide at least one field to update")
	}
	if cfg.Name != nil && strings.TrimSpace(*cfg.Name) == "" {
		return nil, errors.Wrap(errors.ErrBadRequest, "the group name must not be empty")
	}
	group, err := s.ResolveGroup(cfg.Group)
	if err != nil {
		return nil, err
	}
	result, err := s.APIClient.UpdateGroup(group.ID, api.UpdateGroupConfig{Name: cfg.Name, Description: cfg.Description})
	if err != nil {
		return nil, errors.Wrap(err, "unable to update group")
	}
	return s.outputGroup(result.Group, cfg.Json)
}

func (s Service) DeleteGroup(cfg DeleteGroupConfig) (*DeleteGroupResult, error) {
	group, err := s.ResolveGroup(cfg.Group)
	if err != nil {
		return nil, err
	}
	if err := s.confirmDestruction(fmt.Sprintf("Delete group %q (%s)?", group.Name, group.ID), cfg.Yes); err != nil {
		return nil, err
	}
	if err := s.APIClient.DeleteGroup(group.ID); err != nil {
		return nil, errors.Wrap(err, "unable to delete group")
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(group); err != nil {
			return nil, err
		}
	} else {
		fmt.Fprintf(s.Stdout, "Deleted group %q (%s).\n", group.Name, group.ID)
	}
	return group, nil
}

func (s Service) outputGroup(group api.Group, jsonOutput bool) (*GroupInfo, error) {
	info := groupInfo(group)
	if jsonOutput {
		if err := json.NewEncoder(s.Stdout).Encode(info); err != nil {
			return nil, err
		}
	} else {
		s.writeGroups([]GroupInfo{info})
	}
	return &info, nil
}

func (s Service) writeGroups(groups []GroupInfo) {
	if len(groups) == 0 {
		fmt.Fprintln(s.Stdout, "No groups found.")
		return
	}
	w := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tDESCRIPTION")
	for _, group := range groups {
		description := ""
		if group.Description != nil {
			description = *group.Description
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", group.ID, group.Name, description)
	}
	w.Flush()
}
