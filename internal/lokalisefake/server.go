// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

// Package lokalisefake is an in-memory fake of the Lokalise API for tests.
//
// server.go holds the shared state, middleware and helpers; each API area
// registers its routes from its own file (projects.go, ...) via a register*
// method listed in New.
package lokalisefake

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// DefaultTeamID is the team a project is created in when the request omits
// team_id (the token user's current team on the real API).
const DefaultTeamID int64 = 100

const apiPrefix = "/api2"

// Server is an in-memory fake Lokalise API server.
type Server struct {
	srv *httptest.Server

	mu         sync.Mutex
	requests   []string
	failStatus int
	failCount  int

	nextProjectSeq int
	projects       map[string]*project
	// languages holds each project's languages, keyed by project ID.
	languages map[string][]*language
	// contributors holds each project's contributors, keyed by project ID.
	contributors map[string][]*contributor
	// teamUsers holds each team's known users, keyed by team ID. A user
	// becomes a team user the first time they are added as a contributor on
	// one of the team's projects (or via AddTeamUser).
	teamUsers map[int64][]*teamUser
	// nextUserSeq is the next team-user ID to assign; starts at 1000.
	nextUserSeq int64
	// adminRightsReverse, when true, returns admin_rights in reverse order
	// in contributor responses (TestContributor_AdminRightsOrderStable).
	adminRightsReverse bool
	// lowercaseEmails, when true, stores contributor emails lowercased, as
	// a server that normalizes emails would.
	lowercaseEmails bool

	// systemLanguages is the fake's global language catalogue, returned by
	// GET /system/languages.
	systemLanguages []systemLanguageJSON
	// groups holds each team's user groups, keyed by team ID.
	groups map[int64][]*group
	// nextGroupSeq is the next group ID to assign.
	nextGroupSeq int64
}

// project is the fake's stored state for a project.
type project struct {
	ProjectID       string
	ProjectType     string
	Name            string
	Description     string
	TeamID          int64
	BaseLanguageID  int64
	BaseLanguageISO string
}

// language is the fake's stored state for a project language.
type language struct {
	LangID      int64
	LangISO     string
	LangName    string
	IsRTL       bool
	PluralForms []string
}

// contributor is the fake's stored state for a project contributor.
type contributor struct {
	UserID      int64
	Email       string
	Fullname    string
	RoleID      *int64
	AdminRights []string
	Languages   []contributorLanguage
}

// contributorLanguage is a single language assigned to a contributor.
type contributorLanguage struct {
	LangISO    string
	IsWritable bool
}

// teamUser is the fake's stored state for a user known to a team. Users are
// created the first time they're added as a contributor on one of the
// team's projects; Lokalise has no API to create them directly.
type teamUser struct {
	UserID   int64
	Email    string
	Fullname string
	Role     string
}

// group is the fake's stored state for a team user group.
type group struct {
	GroupID     int64
	TeamID      int64
	Name        string
	RoleID      *int64
	AdminRights []string
	Languages   []groupLanguage
	Members     []int64
	Projects    []string
}

// groupLanguage is a single language assigned to a group, with whether it is
// contributable (see client.GroupLanguages).
type groupLanguage struct {
	LangID     int64
	IsWritable bool
}

// New starts a fake server and registers its shutdown with t.Cleanup.
func New(t testing.TB) *Server {
	t.Helper()

	s := &Server{
		projects:        map[string]*project{},
		languages:       map[string][]*language{},
		contributors:    map[string][]*contributor{},
		teamUsers:       map[int64][]*teamUser{},
		nextUserSeq:     1000,
		systemLanguages: append([]systemLanguageJSON(nil), defaultSystemLanguages...),
		groups:          map[int64][]*group{},
	}

	mux := http.NewServeMux()
	s.registerProjects(mux)
	s.registerLanguages(mux)
	s.registerContributors(mux)
	s.registerTeam(mux)
	s.registerGroups(mux)

	s.srv = httptest.NewServer(s.middleware(mux))
	t.Cleanup(s.srv.Close)
	return s
}

// URL returns the API base URL (including the /api2 prefix).
func (s *Server) URL() string {
	return s.srv.URL + apiPrefix
}

// FailNext makes the next n requests fail with status and a standard error
// body, before authentication and routing.
func (s *Server) FailNext(status, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failStatus = status
	s.failCount = n
}

// Requests returns a log of every request received, as "METHOD /path".
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		fail := 0
		if s.failCount > 0 {
			s.failCount--
			fail = s.failStatus
		}
		s.mu.Unlock()

		if fail != 0 {
			writeError(w, fail, http.StatusText(fail))
			return
		}
		if r.Header.Get("X-Api-Token") == "" {
			writeError(w, http.StatusUnauthorized, "Invalid `X-Api-Token` header")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// errorResponse is the standard Lokalise error body.
type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	var body errorResponse
	body.Error.Message = message
	body.Error.Code = status
	writeJSON(w, status, body)
}

// phpArray is a JSON array that, like the real API's PHP backend (per the
// OpenAPI examples), serializes as `{}` when empty.
type phpArray[T any] []T

func (a phpArray[T]) MarshalJSON() ([]byte, error) {
	if len(a) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal([]T(a))
}

// intBool is a boolean serialized as 1/0, as the API does for is_writable
// in list responses (per the OpenAPI examples).
type intBool bool

func (b intBool) MarshalJSON() ([]byte, error) {
	if b {
		return []byte("1"), nil
	}
	return []byte("0"), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSON decodes the request body into v, writing a 400 and returning
// false on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// systemLangID returns a stable system-wide language ID for iso, standing in
// for Lokalise's language catalogue.
func systemLangID(iso string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(iso))
	return int64(h.Sum32()%900000) + 1000
}

// findOrCreateTeamUserLocked returns the existing team user matching email
// (case-insensitive), or creates one with the next sequential user ID.
// Caller holds s.mu.
func (s *Server) findOrCreateTeamUserLocked(teamID int64, email, fullname string) *teamUser {
	for _, u := range s.teamUsers[teamID] {
		if strings.EqualFold(u.Email, email) {
			return u
		}
	}
	u := &teamUser{UserID: s.nextUserSeq, Email: email, Fullname: fullname, Role: "member"}
	s.nextUserSeq++
	s.teamUsers[teamID] = append(s.teamUsers[teamID], u)
	return u
}

// newLanguage builds a language record for iso.
func newLanguage(iso string) *language {
	return &language{
		LangID:      systemLangID(iso),
		LangISO:     iso,
		LangName:    iso,
		PluralForms: []string{"one", "other"},
	}
}
