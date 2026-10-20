// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/url"
)

// Project is a Lokalise project.
type Project struct {
	ID              string
	Name            string
	Description     string
	TeamID          int64
	ProjectType     string
	BaseLanguageISO string
}

// CreateProjectInput is the input to CreateProject.
type CreateProjectInput struct {
	Name        string
	Description string
	// TeamID is the team to create the project in; 0 omits it so Lokalise
	// uses the token user's current team.
	TeamID int64
	// ProjectType is omitted when empty.
	ProjectType     string
	BaseLanguageISO string
}

// UpdateProjectInput is the input to UpdateProject.
type UpdateProjectInput struct {
	Name        string
	Description string
}

type projectWire struct {
	ProjectID       string `json:"project_id"`
	ProjectType     string `json:"project_type"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	TeamID          int64  `json:"team_id"`
	BaseLanguageISO string `json:"base_language_iso"`
}

func (w projectWire) toProject() *Project {
	return &Project{
		ID:              w.ProjectID,
		Name:            w.Name,
		Description:     w.Description,
		TeamID:          w.TeamID,
		ProjectType:     w.ProjectType,
		BaseLanguageISO: w.BaseLanguageISO,
	}
}

type createProjectLanguageWire struct {
	LangISO string `json:"lang_iso"`
}

type createProjectRequest struct {
	Name        string                      `json:"name"`
	Description string                      `json:"description,omitempty"`
	TeamID      int64                       `json:"team_id,omitempty"`
	ProjectType string                      `json:"project_type,omitempty"`
	BaseLangISO string                      `json:"base_lang_iso"`
	Languages   []createProjectLanguageWire `json:"languages"`
}

type updateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func projectPath(projectID string) string {
	return "/projects/" + url.PathEscape(projectID)
}

// CreateProject creates a project with BaseLanguageISO as its only and base
// language.
func (c *HTTPClient) CreateProject(ctx context.Context, in CreateProjectInput) (*Project, error) {
	req := createProjectRequest{
		Name:        in.Name,
		Description: in.Description,
		TeamID:      in.TeamID,
		ProjectType: in.ProjectType,
		BaseLangISO: in.BaseLanguageISO,
		Languages:   []createProjectLanguageWire{{LangISO: in.BaseLanguageISO}},
	}
	var out projectWire
	if _, err := c.do(ctx, http.MethodPost, "/projects", nil, req, &out); err != nil {
		return nil, err
	}
	return out.toProject(), nil
}

// GetProject retrieves a project by ID.
func (c *HTTPClient) GetProject(ctx context.Context, projectID string) (*Project, error) {
	var out projectWire
	if _, err := c.do(ctx, http.MethodGet, projectPath(projectID), nil, nil, &out); err != nil {
		return nil, err
	}
	return out.toProject(), nil
}

// UpdateProject updates a project's name and description.
func (c *HTTPClient) UpdateProject(ctx context.Context, projectID string, in UpdateProjectInput) (*Project, error) {
	req := updateProjectRequest{Name: in.Name, Description: in.Description}
	var out projectWire
	if _, err := c.do(ctx, http.MethodPut, projectPath(projectID), nil, req, &out); err != nil {
		return nil, err
	}
	return out.toProject(), nil
}

// DeleteProject deletes a project.
func (c *HTTPClient) DeleteProject(ctx context.Context, projectID string) error {
	_, err := c.do(ctx, http.MethodDelete, projectPath(projectID), nil, nil, nil)
	return err
}
