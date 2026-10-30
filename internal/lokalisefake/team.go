// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package lokalisefake

import (
	"net/http"
	"strconv"
)

// permissionTemplateJSON is the wire shape of one entry of
// GET /teams/{team_id}/roles' "roles" array (OpenAPI "Role" schema, subset).
type permissionTemplateJSON struct {
	ID   int64  `json:"id"`
	Role string `json:"role"`
}

// defaultPermissionTemplates is the seeded set of permission templates
// every team gets (the fake does not model custom per-team roles).
var defaultPermissionTemplates = []permissionTemplateJSON{
	{ID: 1, Role: "Manager"},
	{ID: 2, Role: "Developer"},
	{ID: 3, Role: "Content creator"},
	{ID: 4, Role: "Reviewer"},
	{ID: 5, Role: "Translator"},
}

// teamUserJSON is the wire shape of one entry of GET /teams/{team_id}/users'
// "team_users" array (OpenAPI "TeamUsers" schema, subset).
type teamUserJSON struct {
	UserID   int64  `json:"user_id"`
	Email    string `json:"email"`
	Fullname string `json:"fullname"`
	Role     string `json:"role"`
}

func (u *teamUser) toJSON() teamUserJSON {
	return teamUserJSON{
		UserID:   u.UserID,
		Email:    u.Email,
		Fullname: u.Fullname,
		Role:     u.Role,
	}
}

func (s *Server) registerTeam(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"/teams/{team_id}/roles", s.handleListRoles)
	mux.HandleFunc("GET "+apiPrefix+"/teams/{team_id}/users", s.handleListTeamUsers)
}

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"team_id": r.PathValue("team_id"),
		"roles":   defaultPermissionTemplates,
	})
}

func (s *Server) handleListTeamUsers(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(r.PathValue("team_id"), 10, 64)
	if err != nil {
		writeNotFound(w)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	users := make([]teamUserJSON, 0, len(s.teamUsers[teamID]))
	for _, u := range s.teamUsers[teamID] {
		users = append(users, u.toJSON())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"team_id":    teamID,
		"team_users": users,
	})
}

// AddTeamUser adds email as a known user of teamID, as if they had already
// joined the team (e.g. via an earlier contributor invite), returning their
// user ID. Calling it again for the same email returns the same ID.
func (s *Server) AddTeamUser(teamID int64, email string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findOrCreateTeamUserLocked(teamID, email, "").UserID
}
