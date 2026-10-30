// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"strconv"
)

// PermissionTemplate is a named set of project permissions (a Lokalise
// "role") that a contributor's role_id can reference. 1-5 are built in
// (Manager, Developer, Content creator, Reviewer, Translator); a team may
// also define custom ones.
type PermissionTemplate struct {
	ID   int64
	Name string
}

// permissionTemplateWire is the wire shape of GET /teams/{team_id}/roles'
// "roles" entries. Per the OpenAPI example, the template ID is under "id"
// and the name under "role".
type permissionTemplateWire struct {
	ID   int64  `json:"id"`
	Role string `json:"role"`
}

func (w permissionTemplateWire) toPermissionTemplate() *PermissionTemplate {
	return &PermissionTemplate{ID: w.ID, Name: w.Role}
}

func rolesPath(teamID int64) string {
	return "/teams/" + strconv.FormatInt(teamID, 10) + "/roles"
}

// ListPermissionTemplates lists the permission templates (roles) available
// to a team.
func (c *HTTPClient) ListPermissionTemplates(ctx context.Context, teamID int64) ([]PermissionTemplate, error) {
	wires, err := listAll[permissionTemplateWire](ctx, c, rolesPath(teamID), "roles")
	if err != nil {
		return nil, err
	}
	out := make([]PermissionTemplate, 0, len(wires))
	for _, w := range wires {
		out = append(out, *w.toPermissionTemplate())
	}
	return out, nil
}
