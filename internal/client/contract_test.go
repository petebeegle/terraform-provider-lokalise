// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/petebeegle/lokalise-provider/internal/client"
)

// The tests in this file are contract tests: each serves a response example
// copied verbatim from the Lokalise OpenAPI spec
// (https://developers.lokalise.com/openapi/lokalise-api-without-branches.yml)
// from testdata/openapi/ through a bare httptest server, and decodes it
// through the client method that calls that endpoint. They pin the client's
// wire structs to the documented shapes independently of lokalisefake, so
// the client and the fake cannot drift away from the API together.

// openAPIExample reads testdata/openapi/name.
func openAPIExample(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "openapi", name))
	if err != nil {
		t.Fatalf("read example %s: %v", name, err)
	}
	return b
}

// serveBody returns a client whose every request is answered 200 with body.
func serveBody(t *testing.T, body []byte) *client.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "tok", client.WithRateLimit(1000, 1000), client.WithMaxRetries(0))
}

// serveExample returns a client whose every request is answered with the
// named OpenAPI example.
func serveExample(t *testing.T, name string) *client.HTTPClient {
	t.Helper()
	return serveBody(t, openAPIExample(t, name))
}

func sortedInt64s(s []int64) []int64 {
	out := append([]int64(nil), s...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func TestContract_LanguageCreate(t *testing.T) {
	c := serveExample(t, "language_create.json")
	l, err := c.CreateLanguage(context.Background(), "p", client.CreateLanguageInput{ISO: "en"})
	if err != nil {
		t.Fatalf("CreateLanguage() error = %v", err)
	}
	if l.ID != 640 || l.ISO != "en" || l.Name != "English" {
		t.Fatalf("CreateLanguage() = %+v, want 640/en/English", l)
	}
}

func TestContract_LanguageGet(t *testing.T) {
	c := serveExample(t, "language_get.json")
	l, err := c.GetLanguage(context.Background(), "p", 640)
	if err != nil {
		t.Fatalf("GetLanguage() error = %v", err)
	}
	if l.ID != 640 || l.ISO != "en" || l.Name != "English" {
		t.Fatalf("GetLanguage() = %+v, want 640/en/English", l)
	}
	if !reflect.DeepEqual(l.PluralForms, []string{"one", "other"}) {
		t.Fatalf("PluralForms = %v", l.PluralForms)
	}
}

func TestContract_LanguageUpdate(t *testing.T) {
	c := serveExample(t, "language_update.json")
	l, err := c.UpdateLanguage(context.Background(), "p", 640, client.UpdateLanguageInput{ISO: "en-US"})
	if err != nil {
		t.Fatalf("UpdateLanguage() error = %v", err)
	}
	if l.ID != 640 || l.ISO != "en-US" {
		t.Fatalf("UpdateLanguage() = %+v, want 640/en-US", l)
	}
	if !reflect.DeepEqual(l.PluralForms, []string{"one", "zero", "few", "other"}) {
		t.Fatalf("PluralForms = %v", l.PluralForms)
	}
}

// TestContract_ContributorList covers "is_writable": 1 (an integer, not a
// bool) in list responses.
func TestContract_ContributorList(t *testing.T) {
	c := serveExample(t, "contributor_list.json")
	cs, err := c.ListContributors(context.Background(), "p")
	if err != nil {
		t.Fatalf("ListContributors() error = %v", err)
	}
	if len(cs) != 1 {
		t.Fatalf("len = %d, want 1", len(cs))
	}
	got := cs[0]
	if got.UserID != 420 || got.Email != "johndoe@mycompany.com" || got.Fullname != "John Doe" {
		t.Fatalf("contributor = %+v", got)
	}
	if !reflect.DeepEqual(got.AdminRights, []string{"keys", "languages"}) {
		t.Fatalf("AdminRights = %v", got.AdminRights)
	}
	want := []client.ContributorLanguage{{ISO: "en", IsWritable: true}, {ISO: "ru", IsWritable: true}}
	if !reflect.DeepEqual(got.Languages, want) {
		t.Fatalf("Languages = %+v, want %+v", got.Languages, want)
	}
}

// TestContract_ContributorCreate covers the singular "contributor" key in the
// POST response and "admin_rights": {} (an empty object for an empty array).
func TestContract_ContributorCreate(t *testing.T) {
	c := serveExample(t, "contributor_create.json")
	got, err := c.CreateContributor(context.Background(), "p", client.ContributorInput{Email: "translator@mycompany.com"})
	if err != nil {
		t.Fatalf("CreateContributor() error = %v", err)
	}
	if got.UserID != 421 || got.Email != "translator@mycompany.com" || got.Fullname != "Mr. Translator" {
		t.Fatalf("contributor = %+v", got)
	}
	if got.RoleID == nil || *got.RoleID != 1 {
		t.Fatalf("RoleID = %v, want 1", got.RoleID)
	}
	if len(got.AdminRights) != 0 {
		t.Fatalf("AdminRights = %v, want empty", got.AdminRights)
	}
	want := []client.ContributorLanguage{{ISO: "en", IsWritable: false}, {ISO: "ru", IsWritable: true}}
	if !reflect.DeepEqual(got.Languages, want) {
		t.Fatalf("Languages = %+v, want %+v", got.Languages, want)
	}
}

// TestContract_ContributorCreate_PluralKey keeps accepting the plural
// "contributors" array, in case the API answers with the request's shape.
func TestContract_ContributorCreate_PluralKey(t *testing.T) {
	c := serveBody(t, []byte(`{"project_id":"p","contributors":[{"user_id":7,"email":"a@b.c","languages":[]}]}`))
	got, err := c.CreateContributor(context.Background(), "p", client.ContributorInput{Email: "a@b.c"})
	if err != nil {
		t.Fatalf("CreateContributor() error = %v", err)
	}
	if got.UserID != 7 || got.Email != "a@b.c" {
		t.Fatalf("contributor = %+v", got)
	}
}

func TestContract_ContributorGet(t *testing.T) {
	c := serveExample(t, "contributor_get.json")
	got, err := c.GetContributor(context.Background(), "p", 421)
	if err != nil {
		t.Fatalf("GetContributor() error = %v", err)
	}
	if got.UserID != 421 || got.Email != "translator@mycompany.com" {
		t.Fatalf("contributor = %+v", got)
	}
	if !reflect.DeepEqual(got.AdminRights, []string{"keys", "languages"}) {
		t.Fatalf("AdminRights = %v", got.AdminRights)
	}
	if len(got.Languages) != 2 {
		t.Fatalf("Languages = %+v, want 2", got.Languages)
	}
}

func TestContract_ContributorUpdate(t *testing.T) {
	c := serveExample(t, "contributor_update.json")
	got, err := c.UpdateContributor(context.Background(), "p", 421, client.ContributorInput{})
	if err != nil {
		t.Fatalf("UpdateContributor() error = %v", err)
	}
	if got.UserID != 421 || got.Email != "translator@mycompany.com" {
		t.Fatalf("contributor = %+v", got)
	}
	if got.RoleID == nil || *got.RoleID != 1 {
		t.Fatalf("RoleID = %v, want 1", got.RoleID)
	}
	want := []client.ContributorLanguage{{ISO: "en", IsWritable: true}, {ISO: "ru", IsWritable: true}}
	if !reflect.DeepEqual(got.Languages, want) {
		t.Fatalf("Languages = %+v, want %+v", got.Languages, want)
	}
}

// TestContract_GroupCreate covers the {"team_id","group":{...}} envelope,
// permissions.role_id, and "projects": {} / "members": {} / "admin_rights":
// {} empty objects.
func TestContract_GroupCreate(t *testing.T) {
	c := serveExample(t, "group_create.json")
	g, err := c.CreateGroup(context.Background(), 444, client.GroupInput{Name: "Proofreaders"})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if g.ID != 50031 || g.TeamID != 444 || g.Name != "Proofreaders" {
		t.Fatalf("group = %+v", g)
	}
	if g.RoleID == nil || *g.RoleID != 1 {
		t.Fatalf("RoleID = %v, want 1", g.RoleID)
	}
	if len(g.AdminRights) != 0 || len(g.MemberIDs) != 0 || len(g.ProjectIDs) != 0 {
		t.Fatalf("group = %+v, want empty admin_rights/members/projects", g)
	}
	if !reflect.DeepEqual(g.Languages.Contributable, []int64{640}) || len(g.Languages.Reference) != 0 {
		t.Fatalf("Languages = %+v, want contributable [640]", g.Languages)
	}
}

// TestContract_GroupGet covers the GET response, which the spec example
// shows unwrapped (no "group" key).
func TestContract_GroupGet(t *testing.T) {
	c := serveExample(t, "group_get.json")
	g, err := c.GetGroup(context.Background(), 444, 50031)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if g.ID != 50031 || g.TeamID != 444 || g.Name != "Proofreaders" {
		t.Fatalf("group = %+v", g)
	}
	if !reflect.DeepEqual(g.Languages.Contributable, []int64{640}) {
		t.Fatalf("Languages = %+v, want contributable [640]", g.Languages)
	}
}

// TestContract_GroupGet_ListItemShape decodes a group as shown in the
// GET /teams/{id}/groups example ("is_writable": 1, non-empty members and
// projects arrays) through GetGroup.
func TestContract_GroupGet_ListItemShape(t *testing.T) {
	var list struct {
		UserGroups []json.RawMessage `json:"user_groups"`
	}
	if err := json.Unmarshal(openAPIExample(t, "group_list.json"), &list); err != nil {
		t.Fatalf("decode group_list.json: %v", err)
	}
	c := serveBody(t, list.UserGroups[0])
	g, err := c.GetGroup(context.Background(), 444, 50031)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if g.ID != 50031 || g.TeamID != 444 {
		t.Fatalf("group = %+v", g)
	}
	if !reflect.DeepEqual(sortedInt64s(g.Languages.Contributable), []int64{597, 640}) {
		t.Fatalf("Languages = %+v, want contributable [597 640]", g.Languages)
	}
	if !reflect.DeepEqual(g.MemberIDs, []int64{22212}) {
		t.Fatalf("MemberIDs = %v", g.MemberIDs)
	}
	if !reflect.DeepEqual(g.ProjectIDs, []string{"598901215bexxx43dcba74.xxx"}) {
		t.Fatalf("ProjectIDs = %v", g.ProjectIDs)
	}
}

// TestContract_GroupUpdate covers the envelope and "languages": {} under
// permissions.
func TestContract_GroupUpdate(t *testing.T) {
	c := serveExample(t, "group_update.json")
	g, err := c.UpdateGroup(context.Background(), 444, 50031, client.GroupInput{Name: "Proofreading admins"})
	if err != nil {
		t.Fatalf("UpdateGroup() error = %v", err)
	}
	if g.ID != 50031 || g.TeamID != 444 || g.Name != "Proofreading admins" {
		t.Fatalf("group = %+v", g)
	}
	if g.RoleID == nil || *g.RoleID != 1 {
		t.Fatalf("RoleID = %v, want 1", g.RoleID)
	}
	if len(g.AdminRights) != 11 {
		t.Fatalf("AdminRights = %v, want 11 entries", g.AdminRights)
	}
	if len(g.Languages.Reference) != 0 || len(g.Languages.Contributable) != 0 {
		t.Fatalf("Languages = %+v, want empty", g.Languages)
	}
}

// TestContract_GroupRoleIDTopLevel covers role_id at the top level of the
// group object, where the TeamUserGroups schema places it (the examples nest
// it under permissions).
func TestContract_GroupRoleIDTopLevel(t *testing.T) {
	c := serveBody(t, []byte(`{"group_id":5,"team_id":1,"name":"g","role_id":3,"permissions":{"admin_rights":{},"languages":{}},"projects":{},"members":{}}`))
	g, err := c.GetGroup(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if g.RoleID == nil || *g.RoleID != 3 {
		t.Fatalf("RoleID = %v, want 3", g.RoleID)
	}
}

func TestContract_GroupMembersAndProjects(t *testing.T) {
	ctx := context.Background()
	if err := serveExample(t, "group_members_add.json").AddGroupMembers(ctx, 444, 50031, []int64{22212}); err != nil {
		t.Fatalf("AddGroupMembers() error = %v", err)
	}
	if err := serveExample(t, "group_projects_add.json").AddGroupProjects(ctx, 444, 50031, []string{"598901215bexxx43dcba74.xxx"}); err != nil {
		t.Fatalf("AddGroupProjects() error = %v", err)
	}
}

// TestContract_RoleList covers the template ID being under "id", not
// "role_id".
func TestContract_RoleList(t *testing.T) {
	c := serveExample(t, "role_list.json")
	roles, err := c.ListPermissionTemplates(context.Background(), 444)
	if err != nil {
		t.Fatalf("ListPermissionTemplates() error = %v", err)
	}
	if len(roles) != 1 || roles[0].ID != 1 || roles[0].Name != "Manager" {
		t.Fatalf("roles = %+v, want [{1 Manager}]", roles)
	}
}

func TestContract_TeamUserList(t *testing.T) {
	c := serveExample(t, "team_user_list.json")
	users, err := c.ListTeamUsers(context.Background(), 18821)
	if err != nil {
		t.Fatalf("ListTeamUsers() error = %v", err)
	}
	if len(users) != 2 || users[0].UserID != 420 || users[0].Email != "jdoe@mycompany.com" || users[1].Role != "member" {
		t.Fatalf("users = %+v", users)
	}
}

func TestContract_Project(t *testing.T) {
	ctx := context.Background()
	check := func(name string, p *client.Project, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: error = %v", name, err)
		}
		if p.ID != "3002780358964f9bab5a92.87762498" || p.TeamID != 12345 || p.ProjectType != "localization_files" || p.Name != "TheApp Project" {
			t.Fatalf("%s: project = %+v", name, p)
		}
	}
	p, err := serveExample(t, "project_create.json").CreateProject(ctx, client.CreateProjectInput{Name: "TheApp Project", BaseLanguageISO: "en-us"})
	check("CreateProject", p, err)
	if p.BaseLanguageISO != "en-us" {
		t.Fatalf("BaseLanguageISO = %q", p.BaseLanguageISO)
	}
	p, err = serveExample(t, "project_get.json").GetProject(ctx, "p")
	check("GetProject", p, err)
	p, err = serveExample(t, "project_update.json").UpdateProject(ctx, "p", client.UpdateProjectInput{Name: "x"})
	check("UpdateProject", p, err)
}
