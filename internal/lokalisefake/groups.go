// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package lokalisefake

import (
	"net/http"
	"strconv"
)

// systemLanguageJSON is the wire shape of one entry of GET
// /system/languages' "languages" array.
type systemLanguageJSON struct {
	LangID  int64  `json:"lang_id"`
	LangISO string `json:"lang_iso"`
}

// defaultSystemLanguages is the fake's seeded global language catalogue.
var defaultSystemLanguages = []systemLanguageJSON{
	{LangID: 640, LangISO: "en"},
	{LangID: 673, LangISO: "fr"},
	{LangID: 666, LangISO: "de"},
}

// groupLanguageJSON is the wire shape of one entry of a group's
// permissions.languages array.
type groupLanguageJSON struct {
	LangID     int64 `json:"lang_id"`
	IsWritable bool  `json:"is_writable"`
}

// groupPermissionsJSON is the "permissions" sub-object of a group response.
// role_id is nested here, as in the OpenAPI examples. Empty arrays are
// serialized as `{}`, as in the OpenAPI examples.
type groupPermissionsJSON struct {
	IsAdmin     bool                        `json:"is_admin"`
	IsReviewer  bool                        `json:"is_reviewer"`
	RoleID      *int64                      `json:"role_id,omitempty"`
	AdminRights phpArray[string]            `json:"admin_rights"`
	Languages   phpArray[groupLanguageJSON] `json:"languages"`
}

// groupJSON is the wire shape of a group (OpenAPI "TeamUserGroups" schema,
// subset). Empty projects/members are serialized as `{}`, as in the
// OpenAPI examples.
type groupJSON struct {
	GroupID     int64                `json:"group_id"`
	TeamID      int64                `json:"team_id"`
	Name        string               `json:"name"`
	Permissions groupPermissionsJSON `json:"permissions"`
	Projects    phpArray[string]     `json:"projects"`
	Members     phpArray[int64]      `json:"members"`
}

func (g *group) toJSON() groupJSON {
	langs := make([]groupLanguageJSON, 0, len(g.Languages))
	for _, l := range g.Languages {
		langs = append(langs, groupLanguageJSON{LangID: l.LangID, IsWritable: l.IsWritable})
	}
	return groupJSON{
		GroupID: g.GroupID,
		TeamID:  g.TeamID,
		Name:    g.Name,
		Permissions: groupPermissionsJSON{
			RoleID:      g.RoleID,
			AdminRights: append([]string(nil), g.AdminRights...),
			Languages:   langs,
		},
		Projects: append([]string(nil), g.Projects...),
		Members:  append([]int64(nil), g.Members...),
	}
}

// groupLanguagesRequestJSON is the "languages" field of the create/update
// request body: two arrays of system language IDs (not the flat
// {lang_id, is_writable} shape the GET response nests under
// permissions.languages). Pointers tell a missing array from an empty one.
type groupLanguagesRequestJSON struct {
	Reference     *[]int64 `json:"reference"`
	Contributable *[]int64 `json:"contributable"`
}

// validationError returns the live API's 400 message for a languages field
// it rejects, or "" if the field is accepted. Observed live: reference must
// be a non-empty array, and contributable must be present (it may be empty).
func (l groupLanguagesRequestJSON) validationError() string {
	if l.Reference == nil || len(*l.Reference) == 0 {
		return "Invalid `languages`.`reference` parameter"
	}
	if l.Contributable == nil {
		return "Invalid `languages`.`contributable` parameter"
	}
	return ""
}

// toGroupLanguages flattens the request into the group's language list. A
// language in both arrays is stored once, as contributable. Observed live:
// a create with an empty contributable array stores no languages at all,
// while an update stores the reference languages.
func (l groupLanguagesRequestJSON) toGroupLanguages(isCreate bool) []groupLanguage {
	contributable := *l.Contributable
	if isCreate && len(contributable) == 0 {
		return nil
	}
	langs := make([]groupLanguage, 0, len(*l.Reference)+len(contributable))
	isContributable := map[int64]bool{}
	for _, id := range contributable {
		isContributable[id] = true
		langs = append(langs, groupLanguage{LangID: id, IsWritable: true})
	}
	for _, id := range *l.Reference {
		if !isContributable[id] {
			langs = append(langs, groupLanguage{LangID: id, IsWritable: false})
		}
	}
	return langs
}

// impliedAdminRight is added by the live API to every group's admin_rights,
// whether the group was given a role_id or an explicit admin_rights list.
const impliedAdminRight = "branches_main_modify"

// withImpliedAdminRight returns rights plus impliedAdminRight, as the live
// API stores them.
func withImpliedAdminRight(rights []string) []string {
	out := append([]string(nil), rights...)
	for _, r := range out {
		if r == impliedAdminRight {
			return out
		}
	}
	return append(out, impliedAdminRight)
}

type groupRequestBody struct {
	Name        string                    `json:"name"`
	IsAdmin     bool                      `json:"is_admin"`
	IsReviewer  bool                      `json:"is_reviewer"`
	RoleID      *int64                    `json:"role_id"`
	AdminRights []string                  `json:"admin_rights"`
	Languages   groupLanguagesRequestJSON `json:"languages"`
}

func (s *Server) registerGroups(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"/system/languages", s.handleListSystemLanguages)
	mux.HandleFunc("POST "+apiPrefix+"/teams/{team_id}/groups", s.handleCreateGroup)
	mux.HandleFunc("GET "+apiPrefix+"/teams/{team_id}/groups/{group_id}", s.handleGetGroup)
	mux.HandleFunc("PUT "+apiPrefix+"/teams/{team_id}/groups/{group_id}", s.handleUpdateGroup)
	mux.HandleFunc("DELETE "+apiPrefix+"/teams/{team_id}/groups/{group_id}", s.handleDeleteGroup)
	mux.HandleFunc("PUT "+apiPrefix+"/teams/{team_id}/groups/{group_id}/members/add", s.handleAddGroupMembers)
	mux.HandleFunc("PUT "+apiPrefix+"/teams/{team_id}/groups/{group_id}/members/remove", s.handleRemoveGroupMembers)
	mux.HandleFunc("PUT "+apiPrefix+"/teams/{team_id}/groups/{group_id}/projects/add", s.handleAddGroupProjects)
	mux.HandleFunc("PUT "+apiPrefix+"/teams/{team_id}/groups/{group_id}/projects/remove", s.handleRemoveGroupProjects)
}

func (s *Server) handleListSystemLanguages(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"languages": s.systemLanguages,
	})
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var body groupRequestBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if msg := body.Languages.validationError(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	teamID, err := strconv.ParseInt(r.PathValue("team_id"), 10, 64)
	if err != nil {
		writeNotFound(w)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextGroupSeq++
	g := &group{
		GroupID:     s.nextGroupSeq,
		TeamID:      teamID,
		Name:        body.Name,
		RoleID:      body.RoleID,
		AdminRights: withImpliedAdminRight(body.AdminRights),
		Languages:   body.Languages.toGroupLanguages(true),
	}
	s.groups[teamID] = append(s.groups[teamID], g)

	writeJSON(w, http.StatusOK, groupEnvelope{TeamID: g.TeamID, Group: g.toJSON()})
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGroupLocked(r.PathValue("team_id"), r.PathValue("group_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	// The OpenAPI example returns the group unwrapped (unlike POST/PUT).
	writeJSON(w, http.StatusOK, g.toJSON())
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	var body groupRequestBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if msg := body.Languages.validationError(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGroupLocked(r.PathValue("team_id"), r.PathValue("group_id"))
	if !ok {
		writeNotFound(w)
		return
	}

	// Full replace of the group's own fields only; members and projects are
	// untouched.
	g.Name = body.Name
	g.RoleID = body.RoleID
	g.AdminRights = withImpliedAdminRight(body.AdminRights)
	g.Languages = body.Languages.toGroupLanguages(false)

	writeJSON(w, http.StatusOK, groupEnvelope{TeamID: g.TeamID, Group: g.toJSON()})
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	teamID, err := strconv.ParseInt(r.PathValue("team_id"), 10, 64)
	if err != nil {
		writeNotFound(w)
		return
	}
	groupID, err := strconv.ParseInt(r.PathValue("group_id"), 10, 64)
	if err != nil {
		writeNotFound(w)
		return
	}
	if !s.deleteGroupLocked(teamID, groupID) {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"team_id":       teamID,
		"group_id":      groupID,
		"group_deleted": true,
	})
}

// findGroupLocked finds a team's group by its string team_id/group_id path
// values. Caller holds s.mu.
func (s *Server) findGroupLocked(teamIDStr, groupIDStr string) (*group, bool) {
	teamID, err := strconv.ParseInt(teamIDStr, 10, 64)
	if err != nil {
		return nil, false
	}
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		return nil, false
	}
	for _, g := range s.groups[teamID] {
		if g.GroupID == groupID {
			return g, true
		}
	}
	return nil, false
}

// deleteGroupLocked removes a group from a team's group list. Caller holds
// s.mu.
func (s *Server) deleteGroupLocked(teamID, groupID int64) bool {
	gs := s.groups[teamID]
	for i, g := range gs {
		if g.GroupID == groupID {
			s.groups[teamID] = append(gs[:i], gs[i+1:]...)
			return true
		}
	}
	return false
}

// DeleteGroupOutOfBand removes a group as if removed in the Lokalise UI,
// bypassing the HTTP handler. The request is not logged.
func (s *Server) DeleteGroupOutOfBand(teamID, groupID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteGroupLocked(teamID, groupID)
}

// SetGroupAdminRightsOutOfBand replaces a group's admin rights, as if
// changed in the Lokalise UI, bypassing the HTTP handler. The request is not
// logged.
func (s *Server) SetGroupAdminRightsOutOfBand(teamID, groupID int64, rights []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range s.groups[teamID] {
		if g.GroupID == groupID {
			g.AdminRights = append([]string(nil), rights...)
		}
	}
}

// AddGroupMemberOutOfBand adds userID to a group's member list, as if added
// via lokalise_team_user_group_member or by hand in the Lokalise UI. The
// request is not logged.
func (s *Server) AddGroupMemberOutOfBand(teamID, groupID, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.findGroupLocked(strconv.FormatInt(teamID, 10), strconv.FormatInt(groupID, 10))
	if !ok {
		return
	}
	g.Members = append(g.Members, userID)
}

// RemoveGroupMemberOutOfBand removes userID from a group's member list, as
// if removed by hand in the Lokalise UI. The request is not logged.
func (s *Server) RemoveGroupMemberOutOfBand(teamID, groupID, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.findGroupLocked(strconv.FormatInt(teamID, 10), strconv.FormatInt(groupID, 10))
	if !ok {
		return
	}
	g.Members = removeInt64(g.Members, userID)
}

// AddGroupProjectOutOfBand adds projectID to a group's project list, as if
// added by hand in the Lokalise UI. The request is not logged.
func (s *Server) AddGroupProjectOutOfBand(teamID, groupID int64, projectID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.findGroupLocked(strconv.FormatInt(teamID, 10), strconv.FormatInt(groupID, 10))
	if !ok {
		return
	}
	g.Projects = append(g.Projects, projectID)
}

// RemoveGroupProjectOutOfBand removes projectID from a group's project list,
// as if removed by hand in the Lokalise UI. The request is not logged.
func (s *Server) RemoveGroupProjectOutOfBand(teamID, groupID int64, projectID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.findGroupLocked(strconv.FormatInt(teamID, 10), strconv.FormatInt(groupID, 10))
	if !ok {
		return
	}
	g.Projects = removeString(g.Projects, projectID)
}

// groupMembersRequestJSON is the body of PUT .../members/add and
// .../members/remove.
type groupMembersRequestJSON struct {
	Users []int64 `json:"users"`
}

// groupProjectsRequestJSON is the body of PUT .../projects/add and
// .../projects/remove.
type groupProjectsRequestJSON struct {
	Projects []string `json:"projects"`
}

// groupEnvelope is the response shape of the create, update and add/remove
// member/project endpoints: the group wrapped under a "group" key,
// alongside team_id (OpenAPI examples).
type groupEnvelope struct {
	TeamID int64     `json:"team_id"`
	Group  groupJSON `json:"group"`
}

func (s *Server) handleAddGroupMembers(w http.ResponseWriter, r *http.Request) {
	var body groupMembersRequestJSON
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGroupLocked(r.PathValue("team_id"), r.PathValue("group_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	for _, id := range body.Users {
		if !containsInt64(g.Members, id) {
			g.Members = append(g.Members, id)
		}
	}
	writeJSON(w, http.StatusOK, groupEnvelope{TeamID: g.TeamID, Group: g.toJSON()})
}

func (s *Server) handleRemoveGroupMembers(w http.ResponseWriter, r *http.Request) {
	var body groupMembersRequestJSON
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGroupLocked(r.PathValue("team_id"), r.PathValue("group_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	for _, id := range body.Users {
		g.Members = removeInt64(g.Members, id)
	}
	writeJSON(w, http.StatusOK, groupEnvelope{TeamID: g.TeamID, Group: g.toJSON()})
}

func (s *Server) handleAddGroupProjects(w http.ResponseWriter, r *http.Request) {
	var body groupProjectsRequestJSON
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGroupLocked(r.PathValue("team_id"), r.PathValue("group_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	for _, id := range body.Projects {
		if !containsString(g.Projects, id) {
			g.Projects = append(g.Projects, id)
		}
	}
	writeJSON(w, http.StatusOK, groupEnvelope{TeamID: g.TeamID, Group: g.toJSON()})
}

func (s *Server) handleRemoveGroupProjects(w http.ResponseWriter, r *http.Request) {
	var body groupProjectsRequestJSON
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGroupLocked(r.PathValue("team_id"), r.PathValue("group_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	for _, id := range body.Projects {
		g.Projects = removeString(g.Projects, id)
	}
	writeJSON(w, http.StatusOK, groupEnvelope{TeamID: g.TeamID, Group: g.toJSON()})
}

func containsInt64(s []int64, v int64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeInt64(s []int64, v int64) []int64 {
	out := s[:0:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeString(s []string, v string) []string {
	out := s[:0:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
