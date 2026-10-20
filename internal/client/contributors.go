// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ContributorLanguage is a single language assigned to a contributor, with
// whether they may write translations in it.
type ContributorLanguage struct {
	ISO        string
	IsWritable bool
}

// Contributor is a user with access to a Lokalise project.
type Contributor struct {
	UserID      int64
	Email       string
	Fullname    string
	RoleID      *int64
	AdminRights []string
	Languages   []ContributorLanguage
}

// ContributorInput is the input to CreateContributor and UpdateContributor.
// Email and Fullname are ignored by UpdateContributor; the API only accepts
// them when inviting a new user.
type ContributorInput struct {
	Email       string
	Fullname    string
	RoleID      *int64
	AdminRights []string
	Languages   []ContributorLanguage
}

type contributorLanguageWire struct {
	LangISO    string `json:"lang_iso"`
	IsWritable bool   `json:"is_writable"`
}

// contributorLanguageRespWire is a contributor language entry as decoded from
// a response. is_writable is a bool on single-contributor responses but 0/1
// in list responses (OpenAPI examples).
type contributorLanguageRespWire struct {
	LangISO    string   `json:"lang_iso"`
	IsWritable flexBool `json:"is_writable"`
}

// contributorWire is a contributor as decoded from a response. admin_rights
// and languages accept `{}` for an empty array (OpenAPI examples).
type contributorWire struct {
	UserID      int64                                  `json:"user_id"`
	Email       string                                 `json:"email"`
	Fullname    string                                 `json:"fullname"`
	RoleID      *int64                                 `json:"role_id"`
	AdminRights flexSlice[string]                      `json:"admin_rights"`
	Languages   flexSlice[contributorLanguageRespWire] `json:"languages"`
}

func (w contributorWire) toContributor() *Contributor {
	langs := make([]ContributorLanguage, 0, len(w.Languages))
	for _, l := range w.Languages {
		langs = append(langs, ContributorLanguage{ISO: l.LangISO, IsWritable: bool(l.IsWritable)})
	}
	return &Contributor{
		UserID:      w.UserID,
		Email:       w.Email,
		Fullname:    w.Fullname,
		RoleID:      w.RoleID,
		AdminRights: []string(w.AdminRights),
		Languages:   langs,
	}
}

func contributorLanguagesWire(langs []ContributorLanguage) []contributorLanguageWire {
	out := make([]contributorLanguageWire, 0, len(langs))
	for _, l := range langs {
		out = append(out, contributorLanguageWire{LangISO: l.ISO, IsWritable: l.IsWritable})
	}
	return out
}

// createContributorWire is one entry of the POST .../contributors bulk
// request body.
type createContributorWire struct {
	Email       string                    `json:"email"`
	Fullname    string                    `json:"fullname,omitempty"`
	RoleID      *int64                    `json:"role_id,omitempty"`
	AdminRights []string                  `json:"admin_rights,omitempty"`
	Languages   []contributorLanguageWire `json:"languages,omitempty"`
}

type createContributorsRequest struct {
	Contributors []createContributorWire `json:"contributors"`
}

// createContributorsResponse is the POST .../contributors response. The
// OpenAPI example returns the created contributor under a singular
// "contributor" key; the plural "contributors" array is also accepted.
type createContributorsResponse struct {
	Contributor  *contributorWire  `json:"contributor"`
	Contributors []contributorWire `json:"contributors"`
}

// contributorResponse is the GET/PUT .../contributors/{user_id} response
// envelope (OpenAPI example: {"project_id": .., "contributor": {..}}).
type contributorResponse struct {
	ProjectID   string          `json:"project_id"`
	Contributor contributorWire `json:"contributor"`
}

// updateContributorRequest is the PUT .../contributors/{user_id} body.
// Email and Fullname have no effect on update, so they are not sent.
type updateContributorRequest struct {
	RoleID      *int64                    `json:"role_id,omitempty"`
	AdminRights []string                  `json:"admin_rights,omitempty"`
	Languages   []contributorLanguageWire `json:"languages"`
}

func contributorsPath(projectID string) string {
	return "/projects/" + url.PathEscape(projectID) + "/contributors"
}

func contributorPath(projectID string, userID int64) string {
	return contributorsPath(projectID) + "/" + strconv.FormatInt(userID, 10)
}

// ListContributors lists every contributor on a project.
func (c *HTTPClient) ListContributors(ctx context.Context, projectID string) ([]Contributor, error) {
	wires, err := listAll[contributorWire](ctx, c, contributorsPath(projectID), "contributors")
	if err != nil {
		return nil, err
	}
	out := make([]Contributor, 0, len(wires))
	for _, w := range wires {
		out = append(out, *w.toContributor())
	}
	return out, nil
}

// CreateContributor adds a single contributor to a project.
func (c *HTTPClient) CreateContributor(ctx context.Context, projectID string, in ContributorInput) (*Contributor, error) {
	req := createContributorsRequest{Contributors: []createContributorWire{{
		Email:       in.Email,
		Fullname:    in.Fullname,
		RoleID:      in.RoleID,
		AdminRights: in.AdminRights,
		Languages:   contributorLanguagesWire(in.Languages),
	}}}
	var out createContributorsResponse
	if _, err := c.do(ctx, http.MethodPost, contributorsPath(projectID), nil, req, &out); err != nil {
		return nil, err
	}
	switch {
	case out.Contributor != nil:
		return out.Contributor.toContributor(), nil
	case len(out.Contributors) > 0:
		return out.Contributors[0].toContributor(), nil
	default:
		return nil, fmt.Errorf("lokalise: create contributor: empty response")
	}
}

// GetContributor retrieves a single contributor by user ID.
func (c *HTTPClient) GetContributor(ctx context.Context, projectID string, userID int64) (*Contributor, error) {
	var out contributorResponse
	if _, err := c.do(ctx, http.MethodGet, contributorPath(projectID, userID), nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Contributor.toContributor(), nil
}

// UpdateContributor replaces a contributor's role/admin rights and language
// list. in.Email and in.Fullname are ignored.
func (c *HTTPClient) UpdateContributor(ctx context.Context, projectID string, userID int64, in ContributorInput) (*Contributor, error) {
	req := updateContributorRequest{
		RoleID:      in.RoleID,
		AdminRights: in.AdminRights,
		Languages:   contributorLanguagesWire(in.Languages),
	}
	var out contributorResponse
	if _, err := c.do(ctx, http.MethodPut, contributorPath(projectID, userID), nil, req, &out); err != nil {
		return nil, err
	}
	return out.Contributor.toContributor(), nil
}

// DeleteContributor removes a contributor from a project.
func (c *HTTPClient) DeleteContributor(ctx context.Context, projectID string, userID int64) error {
	_, err := c.do(ctx, http.MethodDelete, contributorPath(projectID, userID), nil, nil, nil)
	return err
}
