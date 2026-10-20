// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package lokalisefake

import (
	"net/http"
	"strconv"

	"github.com/petebeegle/lokalise-provider/internal/client"
)

// languageJSON is the wire shape of a language (OpenAPI "Languages" schema,
// subset).
type languageJSON struct {
	LangID      int64    `json:"lang_id"`
	LangISO     string   `json:"lang_iso"`
	LangName    string   `json:"lang_name"`
	IsRTL       bool     `json:"is_rtl"`
	PluralForms []string `json:"plural_forms"`
}

func (l *language) toJSON() languageJSON {
	return languageJSON{
		LangID:      l.LangID,
		LangISO:     l.LangISO,
		LangName:    l.LangName,
		IsRTL:       l.IsRTL,
		PluralForms: l.PluralForms,
	}
}

func (l *language) toClientLanguage() client.Language {
	return client.Language{
		ID:          l.LangID,
		ISO:         l.LangISO,
		Name:        l.LangName,
		PluralForms: l.PluralForms,
	}
}

type createLanguagesBody struct {
	Languages []struct {
		LangISO           string   `json:"lang_iso"`
		CustomName        string   `json:"custom_name"`
		CustomPluralForms []string `json:"custom_plural_forms"`
	} `json:"languages"`
}

// updateLanguageBody is the PUT body; absent fields leave the language's
// current value unchanged.
type updateLanguageBody struct {
	LangISO     string    `json:"lang_iso"`
	LangName    *string   `json:"lang_name"`
	PluralForms *[]string `json:"plural_forms"`
}

func (s *Server) registerLanguages(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"/projects/{project_id}/languages", s.handleListLanguages)
	mux.HandleFunc("POST "+apiPrefix+"/projects/{project_id}/languages", s.handleCreateLanguages)
	mux.HandleFunc("GET "+apiPrefix+"/projects/{project_id}/languages/{language_id}", s.handleGetLanguage)
	mux.HandleFunc("PUT "+apiPrefix+"/projects/{project_id}/languages/{language_id}", s.handleUpdateLanguage)
	mux.HandleFunc("DELETE "+apiPrefix+"/projects/{project_id}/languages/{language_id}", s.handleDeleteLanguage)
}

func (s *Server) handleListLanguages(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	projectID := r.PathValue("project_id")
	if _, ok := s.projects[projectID]; !ok {
		writeNotFound(w)
		return
	}
	langs := s.languages[projectID]
	out := make([]languageJSON, 0, len(langs))
	for _, l := range langs {
		out = append(out, l.toJSON())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": projectID,
		"languages":  out,
	})
}

func (s *Server) handleCreateLanguages(w http.ResponseWriter, r *http.Request) {
	var body createLanguagesBody
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	projectID := r.PathValue("project_id")
	if _, ok := s.projects[projectID]; !ok {
		writeNotFound(w)
		return
	}

	created := make([]languageJSON, 0, len(body.Languages))
	for _, in := range body.Languages {
		for _, existing := range s.languages[projectID] {
			if existing.LangISO == in.LangISO {
				writeError(w, http.StatusBadRequest, "language "+in.LangISO+" already exists in project "+projectID)
				return
			}
		}
		l := newLanguage(in.LangISO)
		if in.CustomName != "" {
			l.LangName = in.CustomName
		}
		if len(in.CustomPluralForms) > 0 {
			l.PluralForms = in.CustomPluralForms
		}
		s.languages[projectID] = append(s.languages[projectID], l)
		created = append(created, l.toJSON())
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": projectID,
		"languages":  created,
	})
}

func (s *Server) handleGetLanguage(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.findLanguageLocked(r.PathValue("project_id"), r.PathValue("language_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": r.PathValue("project_id"),
		"language":   l.toJSON(),
	})
}

func (s *Server) handleUpdateLanguage(w http.ResponseWriter, r *http.Request) {
	var body updateLanguageBody
	if !decodeJSON(w, r, &body) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.findLanguageLocked(r.PathValue("project_id"), r.PathValue("language_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	if body.LangISO != "" {
		l.LangISO = body.LangISO
	}
	if body.LangName != nil {
		l.LangName = *body.LangName
	}
	if body.PluralForms != nil {
		l.PluralForms = *body.PluralForms
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": r.PathValue("project_id"),
		"language":   l.toJSON(),
	})
}

func (s *Server) handleDeleteLanguage(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	projectID := r.PathValue("project_id")
	p, ok := s.projects[projectID]
	if !ok {
		writeNotFound(w)
		return
	}
	l, ok := s.findLanguageLocked(projectID, r.PathValue("language_id"))
	if !ok {
		writeNotFound(w)
		return
	}
	if l.LangID == p.BaseLanguageID {
		writeError(w, http.StatusBadRequest, "cannot delete the project's base language")
		return
	}
	s.deleteLanguageLocked(projectID, l.LangID)
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":       projectID,
		"language_id":      l.LangID,
		"language_deleted": true,
	})
}

// findLanguageLocked finds a project's language by its string language_id
// path value. Caller holds s.mu.
func (s *Server) findLanguageLocked(projectID, langIDStr string) (*language, bool) {
	langID, err := strconv.ParseInt(langIDStr, 10, 64)
	if err != nil {
		return nil, false
	}
	for _, l := range s.languages[projectID] {
		if l.LangID == langID {
			return l, true
		}
	}
	return nil, false
}

// deleteLanguageLocked removes a language from a project's language list.
// Caller holds s.mu.
func (s *Server) deleteLanguageLocked(projectID string, langID int64) {
	langs := s.languages[projectID]
	for i, l := range langs {
		if l.LangID == langID {
			s.languages[projectID] = append(langs[:i], langs[i+1:]...)
			return
		}
	}
}

// Languages returns the stored languages for a project.
func (s *Server) Languages(projectID string) []client.Language {
	s.mu.Lock()
	defer s.mu.Unlock()

	langs := s.languages[projectID]
	out := make([]client.Language, 0, len(langs))
	for _, l := range langs {
		out = append(out, l.toClientLanguage())
	}
	return out
}

// DeleteLanguageOutOfBand removes a language as if it were deleted in the
// Lokalise UI, bypassing the base-language guard. The request is not
// logged.
func (s *Server) DeleteLanguageOutOfBand(projectID string, langID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLanguageLocked(projectID, langID)
}
