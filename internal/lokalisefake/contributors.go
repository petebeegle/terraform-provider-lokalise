// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package lokalisefake

import (
	"net/http"
	"strconv"
	"strings"
)

// contributorLanguageJSON is the wire shape of a contributor's language
// entry (OpenAPI "Contributor" schema, subset).
type contributorLanguageJSON struct {
	LangISO    string `json:"lang_iso"`
	IsWritable bool   `json:"is_writable"`
}

// contributorListLanguageJSON is a contributor's language entry as the list
// endpoint returns it: is_writable is 1/0, not a bool (OpenAPI example).
type contributorListLanguageJSON struct {
	LangID     int64   `json:"lang_id"`
	LangISO    string  `json:"lang_iso"`
	IsWritable intBool `json:"is_writable"`
}

// contributorJSON is the wire shape of a contributor (OpenAPI "Contributor"
// schema, subset). Languages is []contributorLanguageJSON, or
// []contributorListLanguageJSON in list responses. An empty admin_rights is
// serialized as `{}`, as in the OpenAPI examples.
type contributorJSON struct {
	UserID      int64            `json:"user_id"`
	Email       string           `json:"email"`
	Fullname    string           `json:"fullname"`
	RoleID      *int64           `json:"role_id"`
	AdminRights phpArray[string] `json:"admin_rights"`
	Languages   any              `json:"languages"`
}

// toListJSON is toJSON with list-endpoint language entries.
func (c *contributor) toListJSON(reverseRights bool) contributorJSON {
	out := c.toJSON(reverseRights)
	langs := make([]contributorListLanguageJSON, 0, len(c.Languages))
	for _, l := range c.Languages {
		langs = append(langs, contributorListLanguageJSON{LangID: systemLangID(l.LangISO), LangISO: l.LangISO, IsWritable: intBool(l.IsWritable)})
	}
	out.Languages = langs
	return out
}

func (c *contributor) toJSON(reverseRights bool) contributorJSON {
	rights := append([]string(nil), c.AdminRights...)
	if reverseRights {
		for i, j := 0, len(rights)-1; i < j; i, j = i+1, j-1 {
			rights[i], rights[j] = rights[j], rights[i]
		}
	}
	langs := make([]contributorLanguageJSON, 0, len(c.Languages))
	for _, l := range c.Languages {
		langs = append(langs, contributorLanguageJSON(l))
	}
	return contributorJSON{
		UserID:      c.UserID,
		Email:       c.Email,
		Fullname:    c.Fullname,
		RoleID:      c.RoleID,
		AdminRights: rights,
		Languages:   langs,
	}
}

type createContributorsBody struct {
	Contributors []struct {
		Email       string                    `json:"email"`
		Fullname    string                    `json:"fullname"`
		RoleID      *int64                    `json:"role_id"`
		AdminRights []string                  `json:"admin_rights"`
		Languages   []contributorLanguageJSON `json:"languages"`
	} `json:"contributors"`
}

type updateContributorBody struct {
	RoleID      *int64                    `json:"role_id"`
	AdminRights []string                  `json:"admin_rights"`
	Languages   []contributorLanguageJSON `json:"languages"`
}

func (s *Server) registerContributors(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"/projects/{project_id}/contributors", s.handleListContributors)
	mux.HandleFunc("POST "+apiPrefix+"/projects/{project_id}/contributors", s.handleCreateContributors)
	mux.HandleFunc("GET "+apiPrefix+"/projects/{project_id}/contributors/{user_id}", s.handleGetContributor)
	mux.HandleFunc("PUT "+apiPrefix+"/projects/{project_id}/contributors/{user_id}", s.handleUpdateContributor)
	mux.HandleFunc("DELETE "+apiPrefix+"/projects/{project_id}/contributors/{user_id}", s.handleDeleteContributor)
}

func (s *Server) handleListContributors(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	projectID := r.PathValue("project_id")
	if _, ok := s.projects[projectID]; !ok {
		writeNotFound(w)
		return
	}
	cs := s.contributors[projectID]
	out := make([]contributorJSON, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.toListJSON(s.adminRightsReverse))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":   projectID,
		"contributors": out,
	})
}

func (s *Server) handleCreateContributors(w http.ResponseWriter, r *http.Request) {
	var body createContributorsBody
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	projectID := r.PathValue("project_id")
	p, ok := s.projects[projectID]
	if !ok {
		writeNotFound(w)
		return
	}

	created := make([]contributorJSON, 0, len(body.Contributors))
	for _, in := range body.Contributors {
		for _, existing := range s.contributors[projectID] {
			if strings.EqualFold(existing.Email, in.Email) {
				writeError(w, http.StatusBadRequest, "contributor "+in.Email+" already exists in project "+projectID)
				return
			}
		}

		u := s.findOrCreateTeamUserLocked(p.TeamID, in.Email, in.Fullname)
		langs := make([]contributorLanguage, 0, len(in.Languages))
		for _, l := range in.Languages {
			langs = append(langs, contributorLanguage(l))
		}
		email := in.Email
		if s.lowercaseEmails {
			email = strings.ToLower(email)
		}
		rec := &contributor{
			UserID:      u.UserID,
			Email:       email,
			Fullname:    in.Fullname,
			RoleID:      in.RoleID,
			AdminRights: append([]string(nil), in.AdminRights...),
			Languages:   langs,
		}
		s.contributors[projectID] = append(s.contributors[projectID], rec)
		created = append(created, rec.toJSON(s.adminRightsReverse))
	}

	// The OpenAPI example returns a single created contributor under a
	// singular "contributor" key.
	resp := map[string]any{"project_id": projectID}
	if len(created) == 1 {
		resp["contributor"] = created[0]
	} else {
		resp["contributors"] = created
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetContributor(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.findContributorLocked(r.PathValue("project_id"), r.PathValue("user_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":  r.PathValue("project_id"),
		"contributor": c.toJSON(s.adminRightsReverse),
	})
}

func (s *Server) handleUpdateContributor(w http.ResponseWriter, r *http.Request) {
	var body updateContributorBody
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.findContributorLocked(r.PathValue("project_id"), r.PathValue("user_id"))
	if !ok {
		writeNotFound(w)
		return
	}

	c.RoleID = body.RoleID
	c.AdminRights = append([]string(nil), body.AdminRights...)
	langs := make([]contributorLanguage, 0, len(body.Languages))
	for _, l := range body.Languages {
		langs = append(langs, contributorLanguage(l))
	}
	c.Languages = langs

	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":  r.PathValue("project_id"),
		"contributor": c.toJSON(s.adminRightsReverse),
	})
}

func (s *Server) handleDeleteContributor(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	projectID := r.PathValue("project_id")
	if _, ok := s.projects[projectID]; !ok {
		writeNotFound(w)
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("user_id"), 10, 64)
	if err != nil {
		writeNotFound(w)
		return
	}
	if !s.deleteContributorLocked(projectID, userID) {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":          projectID,
		"user_id":             userID,
		"contributor_deleted": true,
	})
}

// findContributorLocked finds a project's contributor by its string
// user_id path value. Caller holds s.mu.
func (s *Server) findContributorLocked(projectID, userIDStr string) (*contributor, bool) {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, false
	}
	for _, c := range s.contributors[projectID] {
		if c.UserID == userID {
			return c, true
		}
	}
	return nil, false
}

// deleteContributorLocked removes a contributor from a project's
// contributor list. Caller holds s.mu.
func (s *Server) deleteContributorLocked(projectID string, userID int64) bool {
	cs := s.contributors[projectID]
	for i, c := range cs {
		if c.UserID == userID {
			s.contributors[projectID] = append(cs[:i], cs[i+1:]...)
			return true
		}
	}
	return false
}

// SetAdminRightsOrder makes contributor responses return admin_rights in
// reverse order when reverse is true, simulating a server that doesn't
// preserve configured order (TestContributor_AdminRightsOrderStable).
func (s *Server) SetAdminRightsOrder(reverse bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adminRightsReverse = reverse
}

// SetLowercaseEmails makes the fake store (and so return) contributor
// emails lowercased when lower is true, simulating a server that normalizes
// emails.
func (s *Server) SetLowercaseEmails(lower bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lowercaseEmails = lower
}

// SetContributorFullnameOutOfBand changes a contributor's fullname as if the
// user renamed themselves in Lokalise. The request is not logged.
func (s *Server) SetContributorFullnameOutOfBand(projectID string, userID int64, fullname string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.findContributorLocked(projectID, strconv.FormatInt(userID, 10)); ok {
		c.Fullname = fullname
	}
}

// AddContributorOutOfBand adds a contributor to a project as if added by
// hand in the Lokalise UI, returning the new user ID. The request is not
// logged.
func (s *Server) AddContributorOutOfBand(projectID, email string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := s.projects[projectID]
	teamID := DefaultTeamID
	if p != nil {
		teamID = p.TeamID
	}
	u := s.findOrCreateTeamUserLocked(teamID, email, "")
	s.contributors[projectID] = append(s.contributors[projectID], &contributor{
		UserID: u.UserID,
		Email:  email,
	})
	return u.UserID
}

// DeleteContributorOutOfBand removes a contributor as if removed in the
// Lokalise UI, bypassing the HTTP handler. The request is not logged.
func (s *Server) DeleteContributorOutOfBand(projectID string, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteContributorLocked(projectID, userID)
}
