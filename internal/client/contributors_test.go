// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package client_test

import (
	"context"
	"testing"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
	"github.com/petebeegle/terraform-provider-lokalise/internal/lokalisefake"
)

func TestContributors_CRUD(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	p, err := c.CreateProject(ctx, client.CreateProjectInput{Name: "p", BaseLanguageISO: "en"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := c.CreateLanguage(ctx, p.ID, client.CreateLanguageInput{ISO: "fr"}); err != nil {
		t.Fatalf("CreateLanguage() error = %v", err)
	}

	roleID := int64(5)
	created, err := c.CreateContributor(ctx, p.ID, client.ContributorInput{
		Email:     "a@example.com",
		Fullname:  "A One",
		RoleID:    &roleID,
		Languages: []client.ContributorLanguage{{ISO: "fr", IsWritable: true}},
	})
	if err != nil {
		t.Fatalf("CreateContributor() error = %v", err)
	}
	if created.UserID == 0 || created.Email != "a@example.com" {
		t.Fatalf("created = %+v", created)
	}
	if created.RoleID == nil || *created.RoleID != 5 {
		t.Fatalf("created.RoleID = %v, want 5", created.RoleID)
	}
	if len(created.Languages) != 1 || created.Languages[0].ISO != "fr" || !created.Languages[0].IsWritable {
		t.Fatalf("created.Languages = %+v", created.Languages)
	}

	list, err := c.ListContributors(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListContributors() error = %v", err)
	}
	found := false
	for _, ct := range list {
		if ct.UserID == created.UserID {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListContributors() = %+v, missing created contributor", list)
	}

	updated, err := c.UpdateContributor(ctx, p.ID, created.UserID, client.ContributorInput{
		RoleID:    &roleID,
		Languages: []client.ContributorLanguage{{ISO: "fr", IsWritable: false}},
	})
	if err != nil {
		t.Fatalf("UpdateContributor() error = %v", err)
	}
	if len(updated.Languages) != 1 || updated.Languages[0].IsWritable {
		t.Fatalf("updated.Languages = %+v", updated.Languages)
	}

	got, err := c.GetContributor(ctx, p.ID, created.UserID)
	if err != nil {
		t.Fatalf("GetContributor() error = %v", err)
	}
	if got.UserID != created.UserID {
		t.Fatalf("GetContributor() = %+v", got)
	}

	if err := c.DeleteContributor(ctx, p.ID, created.UserID); err != nil {
		t.Fatalf("DeleteContributor() error = %v", err)
	}
	if _, err := c.GetContributor(ctx, p.ID, created.UserID); !client.IsNotFound(err) {
		t.Fatalf("GetContributor() after delete error = %v, want not found", err)
	}
}

func TestPermissionTemplates_List(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	templates, err := c.ListPermissionTemplates(ctx, lokalisefake.DefaultTeamID)
	if err != nil {
		t.Fatalf("ListPermissionTemplates() error = %v", err)
	}
	names := map[string]int64{}
	for _, tpl := range templates {
		names[tpl.Name] = tpl.ID
	}
	if names["Translator"] != 5 {
		t.Fatalf("ListPermissionTemplates() = %+v, want Translator = 5", templates)
	}
	if names["Manager"] != 1 {
		t.Fatalf("ListPermissionTemplates() = %+v, want Manager = 1", templates)
	}
}
