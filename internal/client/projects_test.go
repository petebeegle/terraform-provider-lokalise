// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package client_test

import (
	"context"
	"errors"
	"testing"

	"github.com/petebeegle/lokalise-provider/internal/client"
	"github.com/petebeegle/lokalise-provider/internal/lokalisefake"
)

func TestProjects_CRUD(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	created, err := c.CreateProject(ctx, client.CreateProjectInput{Name: "p", BaseLanguageISO: "en"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("created.ID is empty")
	}
	if created.TeamID != lokalisefake.DefaultTeamID {
		t.Fatalf("TeamID = %d, want %d", created.TeamID, lokalisefake.DefaultTeamID)
	}
	if created.ProjectType != "localization_files" {
		t.Fatalf("ProjectType = %q, want localization_files", created.ProjectType)
	}
	if created.BaseLanguageISO != "en" || created.Name != "p" {
		t.Fatalf("created = %+v", created)
	}

	got, err := c.GetProject(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if *got != *created {
		t.Fatalf("GetProject() = %+v, want %+v", got, created)
	}

	updated, err := c.UpdateProject(ctx, created.ID, client.UpdateProjectInput{Name: "p2", Description: "d"})
	if err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}
	if updated.Name != "p2" || updated.Description != "d" {
		t.Fatalf("UpdateProject() = %+v", updated)
	}

	if err := c.DeleteProject(ctx, created.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if _, err := c.GetProject(ctx, created.ID); !client.IsNotFound(err) {
		t.Fatalf("GetProject() after delete error = %v, want not found", err)
	}
}

func TestProjects_CreateWithTeamAndType(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	created, err := c.CreateProject(ctx, client.CreateProjectInput{
		Name: "p", Description: "d", TeamID: 555, ProjectType: "paged_documents", BaseLanguageISO: "de",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if created.TeamID != 555 || created.ProjectType != "paged_documents" || created.Description != "d" || created.BaseLanguageISO != "de" {
		t.Fatalf("created = %+v", created)
	}
}

func TestFake_RejectsEmptyToken(t *testing.T) {
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "", client.WithRateLimit(1000, 1000))
	_, err := c.GetProject(context.Background(), "1.abc")
	var apiErr *client.APIError
	if err == nil || !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		t.Fatalf("err = %v, want 401", err)
	}
}
