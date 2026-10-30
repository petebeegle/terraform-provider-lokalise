// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/petebeegle/terraform-provider-lokalise/internal/lokalisefake"
)

const projectLanguageAddr = "lokalise_project_language.test"

// projectAndLanguageConfig renders a lokalise_project resource ("tf-p", base
// language "en") together with a lokalise_project_language resource for iso.
// projectExtra and langExtra are inserted verbatim into the respective
// resource blocks.
func projectAndLanguageConfig(projectExtra, iso, langExtra string) string {
	return projectConfig("tf-p", "en", projectExtra) + fmt.Sprintf(`
resource "lokalise_project_language" "test" {
  project_id = lokalise_project.test.id
  lang_iso   = %q
  %s
}
`, iso, langExtra)
}

func projectIDFromAddr(s *terraform.State, addr string) (string, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return "", fmt.Errorf("%s not in state", addr)
	}
	return rs.Primary.ID, nil
}

func TestProjectLanguage_Lifecycle(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + projectAndLanguageConfig("deletion_protection = false", "fr", "deletion_protection = false"),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectLanguageAddr, "lang_iso", knownvalue.StringExact("fr")),
					stateValue(projectLanguageAddr, "lang_id", knownvalue.NotNull()),
					stateValue(projectLanguageAddr, "deletion_protection", knownvalue.Bool(false)),
				},
				Check: func(s *terraform.State) error {
					rs, ok := s.RootModule().Resources[projectLanguageAddr]
					if !ok {
						return fmt.Errorf("%s not in state", projectLanguageAddr)
					}
					want := rs.Primary.Attributes["project_id"] + ":" + rs.Primary.Attributes["lang_id"]
					if rs.Primary.ID != want {
						return fmt.Errorf("id = %q, want %q", rs.Primary.ID, want)
					}
					return nil
				},
			},
			{
				ResourceName: projectLanguageAddr,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					projectID, err := projectIDFromAddr(s, projectAddr)
					if err != nil {
						return "", err
					}
					return projectID + ":fr", nil
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"deletion_protection"},
			},
			{
				Config: cfg + projectAndLanguageConfig("deletion_protection = false", "fr", `deletion_protection = false
  custom_name = "French (FR)"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(projectLanguageAddr, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectLanguageAddr, "custom_name", knownvalue.StringExact("French (FR)")),
				},
			},
		},
	})
}

func TestProjectLanguage_DeletionProtectionBlocksDestroy(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + projectAndLanguageConfig("deletion_protection = false", "fr", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectLanguageAddr, "deletion_protection", knownvalue.Bool(true)),
				},
			},
			{
				Config:      cfg + projectAndLanguageConfig("deletion_protection = false", "fr", ""),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`Deletion protection enabled`),
			},
			{
				PreConfig: func() {
					for _, r := range fake.Requests() {
						if strings.HasPrefix(r, "DELETE") && strings.Contains(r, "/languages/") {
							t.Fatalf("unexpected request %q while deletion_protection = true", r)
						}
					}
				},
				Config: cfg + projectAndLanguageConfig("deletion_protection = false", "fr", "deletion_protection = false"),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectLanguageAddr, "deletion_protection", knownvalue.Bool(false)),
				},
			},
		},
	})
}

func TestProjectLanguage_BaseLanguageDuplicate(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      cfg + projectAndLanguageConfig("deletion_protection = false", "en", "deletion_protection = false"),
				ExpectError: regexp.MustCompile(`Language already exists`),
			},
		},
	})
}

func TestProjectLanguage_BaseLanguageRemoveIsStateOnly(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	projectCfg := cfg + projectConfig("tf-p", "en", "deletion_protection = false")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: projectCfg,
			},
			{
				Config: projectCfg + `
resource "lokalise_project_language" "test" {
  project_id = lokalise_project.test.id
  lang_iso   = "en"
}
`,
				ResourceName: projectLanguageAddr,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					projectID, err := projectIDFromAddr(s, projectAddr)
					if err != nil {
						return "", err
					}
					return projectID + ":en", nil
				},
				ImportStatePersist: true,
			},
			{
				Config: projectCfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(projectLanguageAddr, plancheck.ResourceActionDestroy),
					},
				},
				Check: func(s *terraform.State) error {
					for _, r := range fake.Requests() {
						if strings.HasPrefix(r, "DELETE") && strings.Contains(r, "/languages/") {
							return fmt.Errorf("unexpected request %q removing the base language", r)
						}
					}
					projectID, err := projectIDFromAddr(s, projectAddr)
					if err != nil {
						return err
					}
					for _, l := range fake.Languages(projectID) {
						if l.ISO == "en" {
							return nil
						}
					}
					return fmt.Errorf("fake lost base language en for project %s", projectID)
				},
			},
		},
	})
}

func TestProjectLanguage_RemovedOutOfBand(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + projectAndLanguageConfig("deletion_protection = false", "fr", "deletion_protection = false")

	var projectID string
	var langID int64

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					rs, ok := s.RootModule().Resources[projectLanguageAddr]
					if !ok {
						return fmt.Errorf("%s not in state", projectLanguageAddr)
					}
					projectID = rs.Primary.Attributes["project_id"]
					id, err := strconv.ParseInt(rs.Primary.Attributes["lang_id"], 10, 64)
					if err != nil {
						return err
					}
					langID = id
					return nil
				},
			},
			{
				PreConfig: func() { fake.DeleteLanguageOutOfBand(projectID, langID) },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(projectLanguageAddr, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

func TestProjectLanguage_ImportBadID(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + projectAndLanguageConfig("deletion_protection = false", "fr", "deletion_protection = false")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				Config:        config,
				ResourceName:  projectLanguageAddr,
				ImportState:   true,
				ImportStateId: "nocolon",
				ExpectError:   regexp.MustCompile(`expected project_id:lang_iso`),
			},
		},
	})
}

// languagePUTs returns the PUT .../languages/{id} requests the fake has
// received.
func languagePUTs(fake *lokalisefake.Server) []string {
	var out []string
	for _, r := range fake.Requests() {
		if strings.HasPrefix(r, "PUT ") && strings.Contains(r, "/languages/") {
			out = append(out, r)
		}
	}
	return out
}

// TestProjectLanguage_DeletionProtectionOnlyChangeSendsNoPUT asserts that
// flipping only deletion_protection plans no change to custom_name or
// plural_forms and sends no PUT (which would otherwise overwrite the
// language's name and plural forms with empty values).
func TestProjectLanguage_DeletionProtectionOnlyChangeSendsNoPUT(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + projectAndLanguageConfig("deletion_protection = false", "fr", ""),
			},
			{
				Config: cfg + projectAndLanguageConfig("deletion_protection = false", "fr", "deletion_protection = false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(projectLanguageAddr, plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue(projectLanguageAddr, tfjsonpath.New("custom_name"), knownvalue.StringExact("fr")),
						plancheck.ExpectKnownValue(projectLanguageAddr, tfjsonpath.New("plural_forms"), knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("one"), knownvalue.StringExact("other"),
						})),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectLanguageAddr, "custom_name", knownvalue.StringExact("fr")),
					stateValue(projectLanguageAddr, "deletion_protection", knownvalue.Bool(false)),
				},
				Check: func(s *terraform.State) error {
					if puts := languagePUTs(fake); len(puts) != 0 {
						return fmt.Errorf("deletion_protection-only change sent %v, want no PUT", puts)
					}
					return nil
				},
			},
		},
	})
}

// TestProjectLanguage_ChangeISOKeepsNameAndPluralForms asserts that changing
// only lang_iso, with custom_name and plural_forms unconfigured, does not
// clear the language's name or plural forms.
func TestProjectLanguage_ChangeISOKeepsNameAndPluralForms(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	var projectID string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + projectAndLanguageConfig("deletion_protection = false", "fr", "deletion_protection = false"),
				Check: func(s *terraform.State) error {
					var err error
					projectID, err = projectIDFromAddr(s, projectAddr)
					return err
				},
			},
			{
				Config: cfg + projectAndLanguageConfig("deletion_protection = false", "fr_CA", "deletion_protection = false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(projectLanguageAddr, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectLanguageAddr, "lang_iso", knownvalue.StringExact("fr_CA")),
					stateValue(projectLanguageAddr, "custom_name", knownvalue.StringExact("fr")),
					stateValue(projectLanguageAddr, "plural_forms", knownvalue.ListExact([]knownvalue.Check{
						knownvalue.StringExact("one"), knownvalue.StringExact("other"),
					})),
				},
				Check: func(s *terraform.State) error {
					for _, l := range fake.Languages(projectID) {
						if l.ISO != "fr_CA" {
							continue
						}
						if l.Name != "fr" || len(l.PluralForms) != 2 {
							return fmt.Errorf("language after ISO change = %+v, want name fr and 2 plural forms", l)
						}
						return nil
					}
					return fmt.Errorf("fr_CA not found in %+v", fake.Languages(projectID))
				},
			},
		},
	})
}
