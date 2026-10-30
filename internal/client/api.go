// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package client

import "context"

// API is the set of Lokalise API operations the provider depends on.
// Methods are added by later tasks as resources and data sources are
// implemented.
type API interface {
	CreateProject(ctx context.Context, in CreateProjectInput) (*Project, error)
	GetProject(ctx context.Context, projectID string) (*Project, error)
	UpdateProject(ctx context.Context, projectID string, in UpdateProjectInput) (*Project, error)
	DeleteProject(ctx context.Context, projectID string) error

	ListLanguages(ctx context.Context, projectID string) ([]Language, error)
	CreateLanguage(ctx context.Context, projectID string, in CreateLanguageInput) (*Language, error)
	GetLanguage(ctx context.Context, projectID string, langID int64) (*Language, error)
	UpdateLanguage(ctx context.Context, projectID string, langID int64, in UpdateLanguageInput) (*Language, error)
	DeleteLanguage(ctx context.Context, projectID string, langID int64) error

	ListSystemLanguages(ctx context.Context) ([]SystemLanguage, error)

	ListContributors(ctx context.Context, projectID string) ([]Contributor, error)
	CreateContributor(ctx context.Context, projectID string, in ContributorInput) (*Contributor, error)
	GetContributor(ctx context.Context, projectID string, userID int64) (*Contributor, error)
	UpdateContributor(ctx context.Context, projectID string, userID int64, in ContributorInput) (*Contributor, error)
	DeleteContributor(ctx context.Context, projectID string, userID int64) error

	ListPermissionTemplates(ctx context.Context, teamID int64) ([]PermissionTemplate, error)

	CreateGroup(ctx context.Context, teamID int64, in GroupInput) (*Group, error)
	GetGroup(ctx context.Context, teamID, groupID int64) (*Group, error)
	UpdateGroup(ctx context.Context, teamID, groupID int64, in GroupInput) (*Group, error)
	DeleteGroup(ctx context.Context, teamID, groupID int64) error

	AddGroupMembers(ctx context.Context, teamID, groupID int64, userIDs []int64) error
	RemoveGroupMembers(ctx context.Context, teamID, groupID int64, userIDs []int64) error
	AddGroupProjects(ctx context.Context, teamID, groupID int64, projectIDs []string) error
	RemoveGroupProjects(ctx context.Context, teamID, groupID int64, projectIDs []string) error

	ListTeamUsers(ctx context.Context, teamID int64) ([]TeamUser, error)
}
