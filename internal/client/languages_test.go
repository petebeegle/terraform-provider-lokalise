// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petebeegle/lokalise-provider/internal/client"
	"github.com/petebeegle/lokalise-provider/internal/lokalisefake"
)

func TestLanguages_CRUD(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	p, err := c.CreateProject(ctx, client.CreateProjectInput{Name: "p", BaseLanguageISO: "en"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	created, err := c.CreateLanguage(ctx, p.ID, client.CreateLanguageInput{ISO: "fr"})
	if err != nil {
		t.Fatalf("CreateLanguage() error = %v", err)
	}
	if created.ISO != "fr" || created.ID == 0 {
		t.Fatalf("created = %+v", created)
	}

	langs, err := c.ListLanguages(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListLanguages() error = %v", err)
	}
	isos := map[string]bool{}
	for _, l := range langs {
		isos[l.ISO] = true
	}
	if !isos["en"] || !isos["fr"] {
		t.Fatalf("ListLanguages() = %+v, want en and fr", langs)
	}

	name := "French"
	updated, err := c.UpdateLanguage(ctx, p.ID, created.ID, client.UpdateLanguageInput{
		ISO: "fr", Name: &name, PluralForms: []string{"one", "other"},
	})
	if err != nil {
		t.Fatalf("UpdateLanguage() error = %v", err)
	}
	if updated.Name != "French" {
		t.Fatalf("UpdateLanguage() = %+v", updated)
	}

	got, err := c.GetLanguage(ctx, p.ID, created.ID)
	if err != nil {
		t.Fatalf("GetLanguage() error = %v", err)
	}
	if got.Name != "French" {
		t.Fatalf("GetLanguage() = %+v", got)
	}

	if err := c.DeleteLanguage(ctx, p.ID, created.ID); err != nil {
		t.Fatalf("DeleteLanguage() error = %v", err)
	}
	if _, err := c.GetLanguage(ctx, p.ID, created.ID); !client.IsNotFound(err) {
		t.Fatalf("GetLanguage() after delete error = %v, want not found", err)
	}
}

func TestLanguages_CreateDuplicateRejected(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	p, err := c.CreateProject(ctx, client.CreateProjectInput{Name: "p", BaseLanguageISO: "en"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if _, err := c.CreateLanguage(ctx, p.ID, client.CreateLanguageInput{ISO: "en"}); err == nil {
		t.Fatal("CreateLanguage() error = nil, want error for duplicate base language")
	}
}

func TestLanguages_DeleteBaseLanguageRejected(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	p, err := c.CreateProject(ctx, client.CreateProjectInput{Name: "p", BaseLanguageISO: "en"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	langs, err := c.ListLanguages(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListLanguages() error = %v", err)
	}
	var baseID int64
	for _, l := range langs {
		if l.ISO == "en" {
			baseID = l.ID
		}
	}
	if baseID == 0 {
		t.Fatalf("base language en not found in %+v", langs)
	}

	if err := c.DeleteLanguage(ctx, p.ID, baseID); err == nil {
		t.Fatal("DeleteLanguage() error = nil, want error for base language")
	}
}

// TestUpdateLanguage_OmitsUnsetFields asserts that UpdateLanguage sends only
// the fields that are set, so an ISO-only update never overwrites the
// language's name or plural forms with empty values.
func TestUpdateLanguage_OmitsUnsetFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"project_id":"p","language":{"lang_id":1,"lang_iso":"fr_CA"}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "tok", client.WithRateLimit(1000, 1000))
	if _, err := c.UpdateLanguage(context.Background(), "p", 1, client.UpdateLanguageInput{ISO: "fr_CA"}); err != nil {
		t.Fatalf("UpdateLanguage() error = %v", err)
	}
	if len(gotBody) != 1 || gotBody["lang_iso"] != "fr_CA" {
		t.Fatalf("request body = %v, want only lang_iso", gotBody)
	}
}
