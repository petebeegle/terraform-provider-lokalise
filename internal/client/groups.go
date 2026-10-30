// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// GroupLanguages are a team user group's two language permission sets:
// reference languages may be viewed, and contributable languages may be
// translated. Both are system language IDs (see ListSystemLanguages), not
// per-project language IDs.
type GroupLanguages struct {
	Reference     []int64
	Contributable []int64
}

// Group is a Lokalise team user group.
type Group struct {
	ID          int64
	TeamID      int64
	Name        string
	RoleID      *int64
	AdminRights []string
	Languages   GroupLanguages
	MemberIDs   []int64
	ProjectIDs  []string
}

// GroupInput is the input to CreateGroup and UpdateGroup. A group's own
// fields (name, role_id/admin_rights, languages) are fully replaced by
// Update; members and projects are managed separately and are untouched by
// it.
type GroupInput struct {
	Name        string
	RoleID      *int64
	AdminRights []string
	Languages   GroupLanguages
}

// groupLanguageWire is one entry of a group's permissions.languages array.
//
// Confirmed against the OpenAPI spec: a group response nests this flat
// shape under permissions.languages. is_writable true means the language is
// contributable, false means reference-only. is_writable is a bool in
// single-group examples but 0/1 in the list example.
type groupLanguageWire struct {
	LangID     int64    `json:"lang_id"`
	IsWritable flexBool `json:"is_writable"`
}

// groupPermissionsWire is the "permissions" sub-object of a group response.
// admin_rights and languages accept `{}` for an empty array (OpenAPI
// examples).
//
// The OpenAPI examples nest role_id here; the TeamUserGroups schema puts it
// at the top level of the group (groupWire.RoleID). Both are accepted.
type groupPermissionsWire struct {
	IsAdmin     bool                         `json:"is_admin"`
	IsReviewer  bool                         `json:"is_reviewer"`
	RoleID      *int64                       `json:"role_id"`
	AdminRights flexSlice[string]            `json:"admin_rights"`
	Languages   flexSlice[groupLanguageWire] `json:"languages"`
}

// groupWire is the wire shape of a group object. projects and members
// accept `{}` for an empty array (OpenAPI examples).
type groupWire struct {
	GroupID     int64                `json:"group_id"`
	TeamID      int64                `json:"team_id"`
	Name        string               `json:"name"`
	RoleID      *int64               `json:"role_id"`
	Permissions groupPermissionsWire `json:"permissions"`
	Projects    flexSlice[string]    `json:"projects"`
	Members     flexSlice[int64]     `json:"members"`
}

// groupResponse decodes a group endpoint's response. Per the OpenAPI
// examples, POST/PUT (and members/projects add/remove) wrap the group as
// {"team_id": .., "group": {..}}, while GET returns the group object
// unwrapped. Both forms are accepted for every endpoint.
type groupResponse struct {
	Group groupWire
}

func (r *groupResponse) UnmarshalJSON(b []byte) error {
	var env struct {
		TeamID int64           `json:"team_id"`
		Group  json.RawMessage `json:"group"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}
	if len(env.Group) > 0 && string(env.Group) != "null" {
		if err := json.Unmarshal(env.Group, &r.Group); err != nil {
			return err
		}
		if r.Group.TeamID == 0 {
			r.Group.TeamID = env.TeamID
		}
		return nil
	}
	return json.Unmarshal(b, &r.Group)
}

func (w groupWire) toGroup() *Group {
	var ref, contrib []int64
	for _, l := range w.Permissions.Languages {
		if l.IsWritable {
			contrib = append(contrib, l.LangID)
		} else {
			ref = append(ref, l.LangID)
		}
	}
	roleID := w.Permissions.RoleID
	if roleID == nil {
		roleID = w.RoleID
	}
	return &Group{
		ID:          w.GroupID,
		TeamID:      w.TeamID,
		Name:        w.Name,
		RoleID:      roleID,
		AdminRights: []string(w.Permissions.AdminRights),
		Languages:   GroupLanguages{Reference: ref, Contributable: contrib},
		MemberIDs:   []int64(w.Members),
		ProjectIDs:  []string(w.Projects),
	}
}

// groupLanguagesRequestWire is the "languages" field of the POST/PUT
// .../groups(/{group_id}) body: confirmed against the OpenAPI spec to be
// two arrays of system language IDs, not the flat {lang_id, is_writable}
// shape the GET response nests under permissions.languages.
type groupLanguagesRequestWire struct {
	Reference     []int64 `json:"reference"`
	Contributable []int64 `json:"contributable"`
}

// groupLanguagesRequest builds the request "languages" field, sending an
// empty array (never null) for an empty set.
//
// The live API rejects an empty reference array with a 400, so an empty
// Reference is sent as the contributable languages instead. That grants
// nothing extra: a contributable language is already readable, and the API
// stores a language listed in both arrays once, as contributable.
func groupLanguagesRequest(langs GroupLanguages) groupLanguagesRequestWire {
	reference := langs.Reference
	if len(reference) == 0 {
		reference = langs.Contributable
	}
	return groupLanguagesRequestWire{
		Reference:     append([]int64{}, reference...),
		Contributable: append([]int64{}, langs.Contributable...),
	}
}

// groupRequest is the POST/PUT .../groups(/{group_id}) body.
type groupRequest struct {
	Name        string                    `json:"name"`
	IsAdmin     bool                      `json:"is_admin"`
	IsReviewer  bool                      `json:"is_reviewer"`
	RoleID      *int64                    `json:"role_id,omitempty"`
	AdminRights []string                  `json:"admin_rights,omitempty"`
	Languages   groupLanguagesRequestWire `json:"languages"`
}

func groupInputToRequest(in GroupInput) groupRequest {
	return groupRequest{
		Name:        in.Name,
		IsAdmin:     false,
		IsReviewer:  false,
		RoleID:      in.RoleID,
		AdminRights: in.AdminRights,
		Languages:   groupLanguagesRequest(in.Languages),
	}
}

func groupsPath(teamID int64) string {
	return "/teams/" + strconv.FormatInt(teamID, 10) + "/groups"
}

func groupPath(teamID, groupID int64) string {
	return groupsPath(teamID) + "/" + strconv.FormatInt(groupID, 10)
}

// CreateGroup creates a team user group.
func (c *HTTPClient) CreateGroup(ctx context.Context, teamID int64, in GroupInput) (*Group, error) {
	req := groupInputToRequest(in)
	var out groupResponse
	if _, err := c.do(ctx, http.MethodPost, groupsPath(teamID), nil, req, &out); err != nil {
		return nil, err
	}
	return out.Group.toGroup(), nil
}

// GetGroup retrieves a single team user group by ID.
func (c *HTTPClient) GetGroup(ctx context.Context, teamID, groupID int64) (*Group, error) {
	var out groupResponse
	if _, err := c.do(ctx, http.MethodGet, groupPath(teamID, groupID), nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Group.toGroup(), nil
}

// UpdateGroup replaces a group's name, role_id/admin_rights and languages.
// It does not affect the group's members or projects.
func (c *HTTPClient) UpdateGroup(ctx context.Context, teamID, groupID int64, in GroupInput) (*Group, error) {
	req := groupInputToRequest(in)
	var out groupResponse
	if _, err := c.do(ctx, http.MethodPut, groupPath(teamID, groupID), nil, req, &out); err != nil {
		return nil, err
	}
	return out.Group.toGroup(), nil
}

// DeleteGroup deletes a team user group.
func (c *HTTPClient) DeleteGroup(ctx context.Context, teamID, groupID int64) error {
	_, err := c.do(ctx, http.MethodDelete, groupPath(teamID, groupID), nil, nil, nil)
	return err
}

// groupMembersRequest is the body of PUT .../members/add and
// .../members/remove.
type groupMembersRequest struct {
	Users []int64 `json:"users"`
}

// groupProjectsRequest is the body of PUT .../projects/add and
// .../projects/remove.
type groupProjectsRequest struct {
	Projects []string `json:"projects"`
}

// AddGroupMembers adds the given users to a group's member list. The
// response envelope wraps the updated group under a "group" key
// (confirmed against the OpenAPI spec), but callers of this method only
// need success/failure, so the body is discarded.
func (c *HTTPClient) AddGroupMembers(ctx context.Context, teamID, groupID int64, userIDs []int64) error {
	req := groupMembersRequest{Users: userIDs}
	_, err := c.do(ctx, http.MethodPut, groupPath(teamID, groupID)+"/members/add", nil, req, nil)
	return err
}

// RemoveGroupMembers removes the given users from a group's member list.
func (c *HTTPClient) RemoveGroupMembers(ctx context.Context, teamID, groupID int64, userIDs []int64) error {
	req := groupMembersRequest{Users: userIDs}
	_, err := c.do(ctx, http.MethodPut, groupPath(teamID, groupID)+"/members/remove", nil, req, nil)
	return err
}

// AddGroupProjects adds the given projects to a group's project list.
func (c *HTTPClient) AddGroupProjects(ctx context.Context, teamID, groupID int64, projectIDs []string) error {
	req := groupProjectsRequest{Projects: projectIDs}
	_, err := c.do(ctx, http.MethodPut, groupPath(teamID, groupID)+"/projects/add", nil, req, nil)
	return err
}

// RemoveGroupProjects removes the given projects from a group's project
// list.
func (c *HTTPClient) RemoveGroupProjects(ctx context.Context, teamID, groupID int64, projectIDs []string) error {
	req := groupProjectsRequest{Projects: projectIDs}
	_, err := c.do(ctx, http.MethodPut, groupPath(teamID, groupID)+"/projects/remove", nil, req, nil)
	return err
}
