// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
	"github.com/petebeegle/terraform-provider-lokalise/internal/lokalisefake"
)

func TestSystemLanguages_List(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	langs, err := c.ListSystemLanguages(ctx)
	if err != nil {
		t.Fatalf("ListSystemLanguages() error = %v", err)
	}
	byISO := map[string]int64{}
	for _, l := range langs {
		byISO[l.ISO] = l.ID
	}
	if byISO["en"] != 640 {
		t.Fatalf("en = %d, want 640", byISO["en"])
	}
	if byISO["fr"] != 673 {
		t.Fatalf("fr = %d, want 673", byISO["fr"])
	}
	if byISO["de"] != 666 {
		t.Fatalf("de = %d, want 666", byISO["de"])
	}
}

// TestCreateGroup_LanguagesRequestShape asserts the exact JSON the client
// sends for group languages: two arrays of language IDs under "reference"
// and "contributable" keys, not the flat {lang_id, is_writable} array the
// GET response nests under permissions.languages. This is captured against
// a bare httptest server (not lokalisefake) so a regression in the client
// can't be masked by a matching regression in the fake's decoding.
func TestCreateGroup_LanguagesRequestShape(t *testing.T) {
	ctx := context.Background()

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"group_id":1,"team_id":100,"name":"Translators","permissions":{}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "tok", client.WithRateLimit(1000, 1000))

	roleID := int64(5)
	_, err := c.CreateGroup(ctx, 100, client.GroupInput{
		Name:   "Translators",
		RoleID: &roleID,
		Languages: client.GroupLanguages{
			Reference:     []int64{640},
			Contributable: []int64{673, 597},
		},
	})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	langs, ok := gotBody["languages"].(map[string]any)
	if !ok {
		t.Fatalf("request body %+v: \"languages\" is not an object", gotBody)
	}
	if _, ok := langs["lang_id"]; ok {
		t.Fatalf("request body languages = %+v, must not be the flat {lang_id, is_writable} shape", langs)
	}

	wantReference := []any{float64(640)}
	wantContributable := []any{float64(673), float64(597)}
	gotReference, _ := langs["reference"].([]any)
	gotContributable, _ := langs["contributable"].([]any)
	if !equalAnySlices(gotReference, wantReference) {
		t.Fatalf("languages.reference = %+v, want %+v", gotReference, wantReference)
	}
	if !equalAnySlices(gotContributable, wantContributable) {
		t.Fatalf("languages.contributable = %+v, want %+v", gotContributable, wantContributable)
	}
}

// TestCreateGroup_EmptyLanguagesSentAsArrays asserts a group with no
// languages sends "reference": [] and "contributable": [], never null.
func TestCreateGroup_EmptyLanguagesSentAsArrays(t *testing.T) {
	ctx := context.Background()

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"team_id":100,"group":{"group_id":1,"team_id":100,"name":"g","permissions":{}}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "tok", client.WithRateLimit(1000, 1000))
	roleID := int64(5)
	for _, call := range []func() error{
		func() error {
			_, err := c.CreateGroup(ctx, 100, client.GroupInput{Name: "g", RoleID: &roleID})
			return err
		},
		func() error {
			_, err := c.UpdateGroup(ctx, 100, 1, client.GroupInput{Name: "g", RoleID: &roleID})
			return err
		},
	} {
		if err := call(); err != nil {
			t.Fatalf("call error = %v", err)
		}
		langs, ok := gotBody["languages"].(map[string]any)
		if !ok {
			t.Fatalf("request body %+v: \"languages\" is not an object", gotBody)
		}
		for _, key := range []string{"reference", "contributable"} {
			v, ok := langs[key].([]any)
			if !ok || len(v) != 0 {
				t.Fatalf("languages.%s = %#v, want []", key, langs[key])
			}
		}
	}
}

// TestGroupRequest_EmptyReferenceSentAsContributable asserts that a group
// with contributable languages but no reference languages sends the
// contributable languages as "reference" too, on create and update: the
// live API rejects an empty reference array.
func TestGroupRequest_EmptyReferenceSentAsContributable(t *testing.T) {
	ctx := context.Background()

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"team_id":100,"group":{"group_id":1,"team_id":100,"name":"g","permissions":{}}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "tok", client.WithRateLimit(1000, 1000))
	roleID := int64(5)
	in := client.GroupInput{Name: "g", RoleID: &roleID, Languages: client.GroupLanguages{Contributable: []int64{673}}}
	for _, call := range []func() error{
		func() error { _, err := c.CreateGroup(ctx, 100, in); return err },
		func() error { _, err := c.UpdateGroup(ctx, 100, 1, in); return err },
	} {
		if err := call(); err != nil {
			t.Fatalf("call error = %v", err)
		}
		langs, _ := gotBody["languages"].(map[string]any)
		gotReference, _ := langs["reference"].([]any)
		gotContributable, _ := langs["contributable"].([]any)
		want := []any{float64(673)}
		if !equalAnySlices(gotReference, want) || !equalAnySlices(gotContributable, want) {
			t.Fatalf("languages = %+v, want reference and contributable both %v", langs, want)
		}
	}
}

func equalAnySlices(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGroups_CRUD(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	roleID := int64(5)
	created, err := c.CreateGroup(ctx, lokalisefake.DefaultTeamID, client.GroupInput{
		Name:   "Translators",
		RoleID: &roleID,
		Languages: client.GroupLanguages{
			Reference:     []int64{640},
			Contributable: []int64{673},
		},
	})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if created.ID == 0 || created.Name != "Translators" {
		t.Fatalf("created = %+v", created)
	}
	if created.RoleID == nil || *created.RoleID != 5 {
		t.Fatalf("created.RoleID = %v, want 5", created.RoleID)
	}
	if len(created.Languages.Reference) != 1 || created.Languages.Reference[0] != 640 {
		t.Fatalf("created.Languages.Reference = %+v", created.Languages.Reference)
	}
	if len(created.Languages.Contributable) != 1 || created.Languages.Contributable[0] != 673 {
		t.Fatalf("created.Languages.Contributable = %+v", created.Languages.Contributable)
	}

	got, err := c.GetGroup(ctx, lokalisefake.DefaultTeamID, created.ID)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if got.ID != created.ID || got.TeamID != lokalisefake.DefaultTeamID {
		t.Fatalf("got = %+v", got)
	}

	fake.AddGroupMemberOutOfBand(lokalisefake.DefaultTeamID, created.ID, 9001)

	updated, err := c.UpdateGroup(ctx, lokalisefake.DefaultTeamID, created.ID, client.GroupInput{
		Name:   "Translators Renamed",
		RoleID: &roleID,
		Languages: client.GroupLanguages{
			Contributable: []int64{640, 673},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGroup() error = %v", err)
	}
	if updated.Name != "Translators Renamed" {
		t.Fatalf("updated.Name = %q", updated.Name)
	}
	if len(updated.MemberIDs) != 1 || updated.MemberIDs[0] != 9001 {
		t.Fatalf("UpdateGroup() must not alter members, got MemberIDs = %+v", updated.MemberIDs)
	}

	if err := c.DeleteGroup(ctx, lokalisefake.DefaultTeamID, created.ID); err != nil {
		t.Fatalf("DeleteGroup() error = %v", err)
	}
	if _, err := c.GetGroup(ctx, lokalisefake.DefaultTeamID, created.ID); !client.IsNotFound(err) {
		t.Fatalf("GetGroup() after delete error = %v, want not found", err)
	}
}

// TestAddRemoveGroupMembers_RequestShape asserts the exact JSON the client
// sends for add/remove group members: {"users":[...]}. Captured against a
// bare httptest server so a client regression can't be masked by a matching
// regression in the fake's decoding.
func TestAddRemoveGroupMembers_RequestShape(t *testing.T) {
	ctx := context.Background()

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"team_id":100,"group":{"group_id":1,"team_id":100,"name":"g","permissions":{}}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "tok", client.WithRateLimit(1000, 1000))

	if err := c.AddGroupMembers(ctx, 100, 1, []int64{22212}); err != nil {
		t.Fatalf("AddGroupMembers() error = %v", err)
	}
	wantUsers := []any{float64(22212)}
	gotUsers, _ := gotBody["users"].([]any)
	if !equalAnySlices(gotUsers, wantUsers) {
		t.Fatalf("request body users = %+v, want %+v", gotUsers, wantUsers)
	}
	if len(gotBody) != 1 {
		t.Fatalf("request body = %+v, want exactly {\"users\":[...]}", gotBody)
	}
}

// TestAddRemoveGroupProjects_RequestShape asserts the exact JSON the client
// sends for add/remove group projects: {"projects":[...]}.
func TestAddRemoveGroupProjects_RequestShape(t *testing.T) {
	ctx := context.Background()

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"team_id":100,"group":{"group_id":1,"team_id":100,"name":"g","permissions":{}}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "tok", client.WithRateLimit(1000, 1000))

	if err := c.RemoveGroupProjects(ctx, 100, 1, []string{"proj1"}); err != nil {
		t.Fatalf("RemoveGroupProjects() error = %v", err)
	}
	wantProjects := []any{"proj1"}
	gotProjects, _ := gotBody["projects"].([]any)
	if !equalAnySlices(gotProjects, wantProjects) {
		t.Fatalf("request body projects = %+v, want %+v", gotProjects, wantProjects)
	}
	if len(gotBody) != 1 {
		t.Fatalf("request body = %+v, want exactly {\"projects\":[...]}", gotBody)
	}
}

func TestGroupMembers_AddRemove(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	created, err := c.CreateGroup(ctx, lokalisefake.DefaultTeamID, client.GroupInput{Name: "G", Languages: client.GroupLanguages{Contributable: []int64{640}}})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	if err := c.AddGroupMembers(ctx, lokalisefake.DefaultTeamID, created.ID, []int64{9001}); err != nil {
		t.Fatalf("AddGroupMembers() error = %v", err)
	}
	// Adding the same member twice is a no-op.
	if err := c.AddGroupMembers(ctx, lokalisefake.DefaultTeamID, created.ID, []int64{9001}); err != nil {
		t.Fatalf("AddGroupMembers() (second) error = %v", err)
	}

	got, err := c.GetGroup(ctx, lokalisefake.DefaultTeamID, created.ID)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if len(got.MemberIDs) != 1 || got.MemberIDs[0] != 9001 {
		t.Fatalf("MemberIDs after add (x2) = %+v, want [9001]", got.MemberIDs)
	}

	if err := c.RemoveGroupMembers(ctx, lokalisefake.DefaultTeamID, created.ID, []int64{9001}); err != nil {
		t.Fatalf("RemoveGroupMembers() error = %v", err)
	}
	// Removing an absent member is a no-op.
	if err := c.RemoveGroupMembers(ctx, lokalisefake.DefaultTeamID, created.ID, []int64{9001}); err != nil {
		t.Fatalf("RemoveGroupMembers() (second) error = %v", err)
	}

	got, err = c.GetGroup(ctx, lokalisefake.DefaultTeamID, created.ID)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if len(got.MemberIDs) != 0 {
		t.Fatalf("MemberIDs after remove = %+v, want empty", got.MemberIDs)
	}
}

func TestGroupProjects_AddRemove(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	created, err := c.CreateGroup(ctx, lokalisefake.DefaultTeamID, client.GroupInput{Name: "G", Languages: client.GroupLanguages{Contributable: []int64{640}}})
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	if err := c.AddGroupProjects(ctx, lokalisefake.DefaultTeamID, created.ID, []string{"proj1"}); err != nil {
		t.Fatalf("AddGroupProjects() error = %v", err)
	}
	// Adding the same project twice is a no-op.
	if err := c.AddGroupProjects(ctx, lokalisefake.DefaultTeamID, created.ID, []string{"proj1"}); err != nil {
		t.Fatalf("AddGroupProjects() (second) error = %v", err)
	}

	got, err := c.GetGroup(ctx, lokalisefake.DefaultTeamID, created.ID)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if len(got.ProjectIDs) != 1 || got.ProjectIDs[0] != "proj1" {
		t.Fatalf("ProjectIDs after add (x2) = %+v, want [proj1]", got.ProjectIDs)
	}

	if err := c.RemoveGroupProjects(ctx, lokalisefake.DefaultTeamID, created.ID, []string{"proj1"}); err != nil {
		t.Fatalf("RemoveGroupProjects() error = %v", err)
	}
	// Removing an absent project is a no-op.
	if err := c.RemoveGroupProjects(ctx, lokalisefake.DefaultTeamID, created.ID, []string{"proj1"}); err != nil {
		t.Fatalf("RemoveGroupProjects() (second) error = %v", err)
	}

	got, err = c.GetGroup(ctx, lokalisefake.DefaultTeamID, created.ID)
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	if len(got.ProjectIDs) != 0 {
		t.Fatalf("ProjectIDs after remove = %+v, want empty", got.ProjectIDs)
	}
}
