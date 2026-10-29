// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package lokalisefake

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
)

const defaultProjectType = "localization_files"

// projectJSON is the wire shape of a project (OpenAPI "Projects" schema,
// subset).
type projectJSON struct {
	ProjectID       string `json:"project_id"`
	ProjectType     string `json:"project_type"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	TeamID          int64  `json:"team_id"`
	BaseLanguageID  int64  `json:"base_language_id"`
	BaseLanguageISO string `json:"base_language_iso"`
}

func (p *project) toJSON() projectJSON {
	return projectJSON{
		ProjectID:       p.ProjectID,
		ProjectType:     p.ProjectType,
		Name:            p.Name,
		Description:     p.Description,
		TeamID:          p.TeamID,
		BaseLanguageID:  p.BaseLanguageID,
		BaseLanguageISO: p.BaseLanguageISO,
	}
}

type createProjectBody struct {
	Name        *string         `json:"name"`
	Description string          `json:"description"`
	TeamID      json.RawMessage `json:"team_id"`
	ProjectType string          `json:"project_type"`
	BaseLangISO string          `json:"base_lang_iso"`
	Languages   []struct {
		LangISO   string `json:"lang_iso"`
		CustomISO string `json:"custom_iso"`
	} `json:"languages"`
}

type updateProjectBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (s *Server) registerProjects(mux *http.ServeMux) {
	mux.HandleFunc("POST "+apiPrefix+"/projects", s.handleCreateProject)
	mux.HandleFunc("GET "+apiPrefix+"/projects/{project_id}", s.handleGetProject)
	mux.HandleFunc("PUT "+apiPrefix+"/projects/{project_id}", s.handleUpdateProject)
	mux.HandleFunc("DELETE "+apiPrefix+"/projects/{project_id}", s.handleDeleteProject)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body createProjectBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Name == nil || *body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	teamID := DefaultTeamID
	if len(body.TeamID) > 0 && string(body.TeamID) != "null" {
		id, err := parseTeamID(body.TeamID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		teamID = id
	}

	projectType := body.ProjectType
	if projectType == "" {
		projectType = defaultProjectType
	}

	// Languages: lang_iso, overridden by custom_iso when set.
	var isos []string
	for _, l := range body.Languages {
		iso := l.LangISO
		if l.CustomISO != "" {
			iso = l.CustomISO
		}
		isos = append(isos, iso)
	}
	baseISO := body.BaseLangISO
	if baseISO == "" {
		if len(isos) > 0 {
			baseISO = isos[0]
		} else {
			baseISO = "en"
		}
	}
	if len(isos) == 0 {
		isos = []string{baseISO}
	}
	found := false
	for _, iso := range isos {
		if iso == baseISO {
			found = true
		}
	}
	if !found {
		writeError(w, http.StatusBadRequest, "base_lang_iso must be in the languages list")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextProjectSeq++
	p := &project{
		ProjectID:       newProjectID(s.nextProjectSeq),
		ProjectType:     projectType,
		Name:            *body.Name,
		Description:     body.Description,
		TeamID:          teamID,
		BaseLanguageID:  systemLangID(baseISO),
		BaseLanguageISO: baseISO,
	}
	s.projects[p.ProjectID] = p
	langs := make([]*language, 0, len(isos))
	for _, iso := range isos {
		langs = append(langs, newLanguage(iso))
	}
	s.languages[p.ProjectID] = langs

	writeJSON(w, http.StatusOK, p.toJSON())
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.projects[r.PathValue("project_id")]
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, p.toJSON())
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	var body updateProjectBody
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.projects[r.PathValue("project_id")]
	if !ok {
		writeNotFound(w)
		return
	}
	if body.Name == nil || *body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	p.Name = *body.Name
	if body.Description != nil {
		p.Description = *body.Description
	}
	writeJSON(w, http.StatusOK, p.toJSON())
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := r.PathValue("project_id")
	if _, ok := s.projects[id]; !ok {
		writeNotFound(w)
		return
	}
	s.deleteProjectLocked(id)
	writeJSON(w, http.StatusOK, map[string]any{"project_id": id, "project_deleted": true})
}

// Project returns the stored project with id.
func (s *Server) Project(id string) (client.Project, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.projects[id]
	if !ok {
		return client.Project{}, false
	}
	return client.Project{
		ID:              p.ProjectID,
		Name:            p.Name,
		Description:     p.Description,
		TeamID:          p.TeamID,
		ProjectType:     p.ProjectType,
		BaseLanguageISO: p.BaseLanguageISO,
	}, true
}

// DeleteProjectOutOfBand removes a project (and its languages) as if it were
// deleted in the Lokalise UI. The request is not logged.
func (s *Server) DeleteProjectOutOfBand(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteProjectLocked(id)
}

func (s *Server) deleteProjectLocked(id string) {
	delete(s.projects, id)
	delete(s.languages, id)
}

func writeNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "Not Found")
}

// newProjectID returns an ID shaped "<n>.<hex>".
func newProjectID(n int) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d.%s", n, hex.EncodeToString(b))
}

// parseTeamID accepts team_id as a JSON number or a numeric string.
func parseTeamID(raw json.RawMessage) (int64, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if id, err := n.Int64(); err == nil {
			return id, nil
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			return id, nil
		}
	}
	return 0, fmt.Errorf("invalid team_id %s", raw)
}
