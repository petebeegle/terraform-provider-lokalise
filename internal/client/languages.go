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

// Language is a language added to a Lokalise project.
type Language struct {
	ID          int64
	ISO         string
	Name        string
	PluralForms []string
}

// CreateLanguageInput is the input to CreateLanguage.
type CreateLanguageInput struct {
	ISO         string
	CustomName  string
	PluralForms []string
}

// UpdateLanguageInput is the input to UpdateLanguage. Only set fields are
// sent: an empty ISO, a nil Name and a nil PluralForms are omitted, so the
// API keeps their current values.
type UpdateLanguageInput struct {
	ISO         string
	Name        *string
	PluralForms []string
}

type languageWire struct {
	LangID      int64             `json:"lang_id"`
	LangISO     string            `json:"lang_iso"`
	LangName    string            `json:"lang_name"`
	PluralForms flexSlice[string] `json:"plural_forms"`
}

func (w languageWire) toLanguage() *Language {
	return &Language{
		ID:          w.LangID,
		ISO:         w.LangISO,
		Name:        w.LangName,
		PluralForms: []string(w.PluralForms),
	}
}

// languageResponse is the GET/PUT .../languages/{lang_id} response envelope
// (OpenAPI example: {"project_id": .., "language": {..}}).
type languageResponse struct {
	ProjectID string       `json:"project_id"`
	Language  languageWire `json:"language"`
}

type createLanguageWire struct {
	LangISO           string   `json:"lang_iso"`
	CustomName        string   `json:"custom_name,omitempty"`
	CustomPluralForms []string `json:"custom_plural_forms,omitempty"`
}

type createLanguagesRequest struct {
	Languages []createLanguageWire `json:"languages"`
}

// createLanguagesResponse is the POST .../languages response. A language
// the API refuses (already added, unknown ISO code) is reported in Errors
// of a 200 response, not as an HTTP error.
type createLanguagesResponse struct {
	Languages []languageWire        `json:"languages"`
	Errors    []createLanguageError `json:"errors"`
}

type createLanguageError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type updateLanguageRequest struct {
	LangISO     string   `json:"lang_iso,omitempty"`
	LangName    *string  `json:"lang_name,omitempty"`
	PluralForms []string `json:"plural_forms,omitempty"`
}

func languagesPath(projectID string) string {
	return "/projects/" + url.PathEscape(projectID) + "/languages"
}

func languagePath(projectID string, langID int64) string {
	return languagesPath(projectID) + "/" + strconv.FormatInt(langID, 10)
}

// ListLanguages lists every language in a project.
func (c *HTTPClient) ListLanguages(ctx context.Context, projectID string) ([]Language, error) {
	wires, err := listAll[languageWire](ctx, c, languagesPath(projectID), "languages")
	if err != nil {
		return nil, err
	}
	langs := make([]Language, 0, len(wires))
	for _, w := range wires {
		langs = append(langs, *w.toLanguage())
	}
	return langs, nil
}

// CreateLanguage adds a single language to a project.
func (c *HTTPClient) CreateLanguage(ctx context.Context, projectID string, in CreateLanguageInput) (*Language, error) {
	req := createLanguagesRequest{Languages: []createLanguageWire{{
		LangISO:           in.ISO,
		CustomName:        in.CustomName,
		CustomPluralForms: in.PluralForms,
	}}}
	var out createLanguagesResponse
	if _, err := c.do(ctx, http.MethodPost, languagesPath(projectID), nil, req, &out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, &APIError{StatusCode: out.Errors[0].Code, Message: out.Errors[0].Message}
	}
	if len(out.Languages) == 0 {
		return nil, fmt.Errorf("lokalise: create language: empty response")
	}
	return out.Languages[0].toLanguage(), nil
}

// GetLanguage retrieves a single language by ID.
func (c *HTTPClient) GetLanguage(ctx context.Context, projectID string, langID int64) (*Language, error) {
	var out languageResponse
	if _, err := c.do(ctx, http.MethodGet, languagePath(projectID, langID), nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Language.toLanguage(), nil
}

// UpdateLanguage updates the set fields of in on a language.
func (c *HTTPClient) UpdateLanguage(ctx context.Context, projectID string, langID int64, in UpdateLanguageInput) (*Language, error) {
	req := updateLanguageRequest{LangISO: in.ISO, LangName: in.Name, PluralForms: in.PluralForms}
	var out languageResponse
	if _, err := c.do(ctx, http.MethodPut, languagePath(projectID, langID), nil, req, &out); err != nil {
		return nil, err
	}
	return out.Language.toLanguage(), nil
}

// DeleteLanguage deletes a language from a project.
func (c *HTTPClient) DeleteLanguage(ctx context.Context, projectID string, langID int64) error {
	_, err := c.do(ctx, http.MethodDelete, languagePath(projectID, langID), nil, nil, nil)
	return err
}

// SystemLanguage is a language known to Lokalise globally (not scoped to any
// project), as listed by GET /system/languages. Its ID is the language ID
// space used by lokalise_team_user_group's languages attribute, distinct
// from a project's own per-project language IDs (Language.ID).
type SystemLanguage struct {
	ID  int64
	ISO string
}

type systemLanguageWire struct {
	LangID  int64  `json:"lang_id"`
	LangISO string `json:"lang_iso"`
}

func (w systemLanguageWire) toSystemLanguage() *SystemLanguage {
	return &SystemLanguage{ID: w.LangID, ISO: w.LangISO}
}

// ListSystemLanguages lists every language known to Lokalise.
func (c *HTTPClient) ListSystemLanguages(ctx context.Context) ([]SystemLanguage, error) {
	wires, err := listAll[systemLanguageWire](ctx, c, "/system/languages", "languages")
	if err != nil {
		return nil, err
	}
	out := make([]SystemLanguage, 0, len(wires))
	for _, w := range wires {
		out = append(out, *w.toSystemLanguage())
	}
	return out, nil
}
