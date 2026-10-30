// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
	"github.com/petebeegle/terraform-provider-lokalise/internal/lokalisefake"
)

const projectAddr = "lokalise_project.test"

// projectConfig renders a lokalise_project resource block. extra is inserted
// verbatim inside the block (e.g. `deletion_protection = false`).
func projectConfig(name, baseISO, extra string) string {
	return fmt.Sprintf(`
resource "lokalise_project" "test" {
  name              = %q
  base_language_iso = %q
  %s
}
`, name, baseISO, extra)
}

func stateValue(addr, attr string, v knownvalue.Check) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(addr, tfjsonpath.New(attr), v)
}

// projectIDFromState returns the id attribute of projectAddr in s.
func projectIDFromState(s *terraform.State) (string, error) {
	rs, ok := s.RootModule().Resources[projectAddr]
	if !ok {
		return "", fmt.Errorf("%s not in state", projectAddr)
	}
	return rs.Primary.ID, nil
}

func TestProject_Lifecycle(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + projectConfig("tf-p", "en", "deletion_protection = false"),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "name", knownvalue.StringExact("tf-p")),
					stateValue(projectAddr, "description", knownvalue.StringExact("")),
					stateValue(projectAddr, "project_type", knownvalue.StringExact("localization_files")),
					stateValue(projectAddr, "team_id", knownvalue.Int64Exact(lokalisefake.DefaultTeamID)),
					stateValue(projectAddr, "base_language_iso", knownvalue.StringExact("en")),
					stateValue(projectAddr, "deletion_protection", knownvalue.Bool(false)),
					stateValue(projectAddr, "id", knownvalue.NotNull()),
				},
			},
			{
				ResourceName:            projectAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"deletion_protection"},
			},
			{
				Config: cfg + projectConfig("tf-p2", "en", "deletion_protection = false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(projectAddr, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "name", knownvalue.StringExact("tf-p2")),
				},
			},
			{
				Config: cfg + projectConfig("tf-p2", "de", "deletion_protection = false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(projectAddr, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "base_language_iso", knownvalue.StringExact("de")),
				},
			},
		},
	})
}

func TestProject_DeletionProtectionBlocksDestroy(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + projectConfig("tf-p", "en", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "deletion_protection", knownvalue.Bool(true)),
				},
			},
			{
				Config:      cfg + projectConfig("tf-p", "en", ""),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`Deletion protection enabled`),
			},
			{
				PreConfig: func() {
					for _, r := range fake.Requests() {
						if strings.HasPrefix(r, "DELETE /api2/projects/") {
							t.Fatalf("unexpected request %q while deletion_protection = true", r)
						}
					}
				},
				Config: cfg + projectConfig("tf-p", "en", "deletion_protection = false"),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "deletion_protection", knownvalue.Bool(false)),
				},
			},
		},
	})
}

func TestProject_ImportSetsDeletionProtectionTrue(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	c := client.New(fake.URL(), "tok")
	p, err := c.CreateProject(context.Background(), client.CreateProjectInput{Name: "existing", BaseLanguageISO: "en"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             cfg + projectConfig("existing", "en", ""),
				ResourceName:       projectAddr,
				ImportState:        true,
				ImportStateId:      p.ID,
				ImportStatePersist: true,
			},
			{
				Config: cfg + projectConfig("existing", "en", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "deletion_protection", knownvalue.Bool(true)),
					stateValue(projectAddr, "id", knownvalue.StringExact(p.ID)),
				},
			},
			{
				Config: cfg + projectConfig("existing", "en", "deletion_protection = false"),
			},
		},
	})
}

// TestProject_ImportNonDefaultTypeOmittedPlansNothing asserts that importing
// a project whose type is not the API default, with a config that omits
// project_type, plans no replacement.
func TestProject_ImportNonDefaultTypeOmittedPlansNothing(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	c := client.New(fake.URL(), "tok")
	p, err := c.CreateProject(context.Background(), client.CreateProjectInput{Name: "existing", BaseLanguageISO: "en", ProjectType: "marketing"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             cfg + projectConfig("existing", "en", ""),
				ResourceName:       projectAddr,
				ImportState:        true,
				ImportStateId:      p.ID,
				ImportStatePersist: true,
			},
			{
				Config: cfg + projectConfig("existing", "en", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "project_type", knownvalue.StringExact("marketing")),
				},
			},
			{
				Config: cfg + projectConfig("existing", "en", "deletion_protection = false"),
			},
		},
	})
}

// TestProject_TypeDefaultsServerSide asserts that omitting project_type on
// create yields the API's default type in state.
func TestProject_TypeDefaultsServerSide(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + projectConfig("tf-p", "en", "deletion_protection = false")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "project_type", knownvalue.StringExact("localization_files")),
				},
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestProject_RemovedOutOfBand(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + projectConfig("tf-p", "en", "deletion_protection = false")
	var id string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					var err error
					id, err = projectIDFromState(s)
					return err
				},
			},
			{
				PreConfig: func() { fake.DeleteProjectOutOfBand(id) },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(projectAddr, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

func TestProject_RetriesThroughFaults(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { fake.FailNext(429, 2) },
				Config:    cfg + projectConfig("tf-p", "en", "deletion_protection = false"),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "name", knownvalue.StringExact("tf-p")),
				},
			},
		},
	})
}

func TestProvider_MissingToken(t *testing.T) {
	t.Setenv("LOKALISE_API_TOKEN", "")
	fake := lokalisefake.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "lokalise" {
  api_url = %q
}
`, fake.URL()) + projectConfig("tf-p", "en", "deletion_protection = false"),
				ExpectError: regexp.MustCompile(`Missing Lokalise API token`),
			},
		},
	})
}

func TestProvider_TokenFromEnv(t *testing.T) {
	t.Setenv("LOKALISE_API_TOKEN", "env-token")
	fake := lokalisefake.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "lokalise" {
  api_url = %q
}
`, fake.URL()) + projectConfig("tf-p", "en", "deletion_protection = false"),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(projectAddr, "id", knownvalue.NotNull()),
				},
			},
		},
	})
}
