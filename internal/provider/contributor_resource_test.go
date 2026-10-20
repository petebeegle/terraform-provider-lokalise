// Copyright IBM Corp. 2021, 2026
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

	"github.com/petebeegle/lokalise-provider/internal/lokalisefake"
)

const contributorAddr = "lokalise_contributor.test"

// contributorFixture renders a project ("tf-p", base language "en"), a
// lokalise_project_language "fr", and (unless omitted by the caller)
// relies on the caller to append a lokalise_contributor block. roleOrRights
// is inserted verbatim into the contributor block (e.g. `role_id = 5` or
// `admin_rights = ["upload"]`), and languages is the contributor's
// `languages` block contents.
func contributorFixture(roleOrRights, languages string) string {
	return projectConfig("tf-p", "en", "deletion_protection = false") + `
resource "lokalise_project_language" "fr" {
  project_id          = lokalise_project.test.id
  lang_iso            = "fr"
  deletion_protection = false
}

resource "lokalise_contributor" "test" {
  project_id = lokalise_project.test.id
  email      = "a@example.com"
  ` + roleOrRights + `
  languages = [
    ` + languages + `
  ]
  depends_on = [lokalise_project_language.fr]
}
`
}

func contributorProjectID(s *terraform.State) (string, error) {
	rs, ok := s.RootModule().Resources[projectAddr]
	if !ok {
		return "", fmt.Errorf("%s not in state", projectAddr)
	}
	return rs.Primary.ID, nil
}

func TestContributor_Lifecycle(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	translatorLookup := `
data "lokalise_permission_template" "translator" {
  team_id = lokalise_project.test.team_id
  name    = "Translator"
}
`

	withRole := cfg + translatorLookup + contributorFixture(
		"role_id = data.lokalise_permission_template.translator.role_id",
		`{ lang_iso = "fr", is_writable = true }`,
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: withRole,
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(contributorAddr, "role_id", knownvalue.Int64Exact(5)),
					stateValue(contributorAddr, "user_id", knownvalue.NotNull()),
					stateValue(contributorAddr, "admin_rights", knownvalue.Null()),
					stateValue(contributorAddr, "email", knownvalue.StringExact("a@example.com")),
				},
			},
			{
				ResourceName: contributorAddr,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					projectID, err := contributorProjectID(s)
					if err != nil {
						return "", err
					}
					return projectID + ":a@example.com", nil
				},
				ImportStateVerify: true,
			},
			{
				Config: withRole,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg + translatorLookup + contributorFixture(
					"role_id = data.lokalise_permission_template.translator.role_id",
					`{ lang_iso = "fr", is_writable = false }`,
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(contributorAddr, plancheck.ResourceActionUpdate),
					},
				},
			},
		},
	})
}

func TestContributor_AdminRightsOrderStable(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + contributorFixture(
		`admin_rights = ["upload", "download", "keys"]`,
		`{ lang_iso = "fr", is_writable = true }`,
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(contributorAddr, "role_id", knownvalue.Null()),
				},
			},
			{
				PreConfig: func() { fake.SetAdminRightsOrder(true) },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestContributor_RoleAndRightsConflict(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + contributorFixture(
					`role_id      = 5
  admin_rights = ["upload"]`,
					`{ lang_iso = "fr", is_writable = true }`,
				),
				ExpectError: regexp.MustCompile(`(?s)role_id.*admin_rights|exactly one`),
			},
		},
	})
}

func TestContributor_AlreadyExists(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	projectCfg := cfg + projectConfig("tf-p", "en", "deletion_protection = false") + `
resource "lokalise_project_language" "fr" {
  project_id          = lokalise_project.test.id
  lang_iso            = "fr"
  deletion_protection = false
}
`
	var projectID string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: projectCfg,
				Check: func(s *terraform.State) error {
					id, err := contributorProjectID(s)
					if err != nil {
						return err
					}
					projectID = id
					return nil
				},
			},
			{
				PreConfig: func() { fake.AddContributorOutOfBand(projectID, "a@example.com") },
				Config: cfg + contributorFixture(
					"role_id = 5",
					`{ lang_iso = "fr", is_writable = true }`,
				),
				ExpectError: regexp.MustCompile(`Contributor already exists`),
			},
		},
	})
}

func TestContributor_AlreadyExistsCaseInsensitive(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	projectCfg := cfg + projectConfig("tf-p", "en", "deletion_protection = false") + `
resource "lokalise_project_language" "fr" {
  project_id          = lokalise_project.test.id
  lang_iso            = "fr"
  deletion_protection = false
}
`
	var projectID string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: projectCfg,
				Check: func(s *terraform.State) error {
					id, err := contributorProjectID(s)
					if err != nil {
						return err
					}
					projectID = id
					return nil
				},
			},
			{
				// The out-of-band contributor is added with a differently
				// cased email than the config's "a@example.com"; the
				// duplicate check must still catch it.
				PreConfig: func() { fake.AddContributorOutOfBand(projectID, "A@Example.com") },
				Config: cfg + contributorFixture(
					"role_id = 5",
					`{ lang_iso = "fr", is_writable = true }`,
				),
				ExpectError: regexp.MustCompile(`Contributor already exists`),
			},
		},
	})
}

func TestContributor_ImportCaseInsensitiveEmail(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + contributorFixture(
		"role_id = 5",
		`{ lang_iso = "fr", is_writable = true }`,
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				// Creates the contributor with email "a@example.com".
				Config: config,
			},
			{
				ResourceName: contributorAddr,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					projectID, err := contributorProjectID(s)
					if err != nil {
						return "", err
					}
					// Mismatched case from the stored "a@example.com" -
					// resolution must still find it.
					return projectID + ":A@EXAMPLE.COM", nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

func TestContributor_RemovedOutOfBand(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + contributorFixture(
		"role_id = 5",
		`{ lang_iso = "fr", is_writable = true }`,
	)

	var projectID string
	var userID int64

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					rs, ok := s.RootModule().Resources[contributorAddr]
					if !ok {
						return fmt.Errorf("%s not in state", contributorAddr)
					}
					projectID = rs.Primary.Attributes["project_id"]
					id, err := strconv.ParseInt(rs.Primary.Attributes["user_id"], 10, 64)
					if err != nil {
						return err
					}
					userID = id
					return nil
				},
			},
			{
				PreConfig: func() { fake.DeleteContributorOutOfBand(projectID, userID) },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(contributorAddr, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

func TestContributor_ImportBadID(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + contributorFixture(
		"role_id = 5",
		`{ lang_iso = "fr", is_writable = true }`,
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				Config:        config,
				ResourceName:  contributorAddr,
				ImportState:   true,
				ImportStateId: "nocolon",
				ExpectError:   regexp.MustCompile(`expected project_id:email`),
			},
		},
	})
}

// contributorIDs extracts project_id and user_id from the contributor in s.
func contributorIDs(s *terraform.State) (string, int64, error) {
	rs, ok := s.RootModule().Resources[contributorAddr]
	if !ok {
		return "", 0, fmt.Errorf("%s not in state", contributorAddr)
	}
	id, err := strconv.ParseInt(rs.Primary.Attributes["user_id"], 10, 64)
	return rs.Primary.Attributes["project_id"], id, err
}

// TestContributor_APILowercasesEmail asserts that a mixed-case configured
// email the API returns lowercased neither errors on create nor plans a
// replacement.
func TestContributor_APILowercasesEmail(t *testing.T) {
	fake := lokalisefake.New(t)
	fake.SetLowercaseEmails(true)
	cfg := providerConfig(fake.URL())
	config := cfg + strings.Replace(contributorFixture(
		"role_id = 5",
		`{ lang_iso = "fr", is_writable = true }`,
	), `"a@example.com"`, `"A.Person@Example.com"`, 1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(contributorAddr, "email", knownvalue.StringExact("A.Person@Example.com")),
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

// TestContributor_APIFullnameDiffers asserts that a fullname the API reports
// differently from config (e.g. an existing user's own name) plans no
// change: fullname only matters when inviting.
func TestContributor_APIFullnameDiffers(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + strings.Replace(contributorFixture(
		"role_id = 5",
		`{ lang_iso = "fr", is_writable = true }`,
	), `email      = "a@example.com"`, `email      = "a@example.com"
  fullname   = "Alice"`, 1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					projectID, userID, err := contributorIDs(s)
					if err != nil {
						return err
					}
					fake.SetContributorFullnameOutOfBand(projectID, userID, "Alice Smith")
					return nil
				},
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(contributorAddr, "fullname", knownvalue.StringExact("Alice")),
				},
			},
		},
	})
}

// TestContributor_FullnameChangeIsInPlace asserts changing fullname updates
// in place (no replacement).
func TestContributor_FullnameChangeIsInPlace(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	withName := func(name string) string {
		return cfg + strings.Replace(contributorFixture(
			"role_id = 5",
			`{ lang_iso = "fr", is_writable = true }`,
		), `email      = "a@example.com"`, `email      = "a@example.com"
  fullname   = "`+name+`"`, 1)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{Config: withName("Alice")},
			{
				Config: withName("Alice B"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(contributorAddr, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(contributorAddr, "fullname", knownvalue.StringExact("Alice B")),
				},
			},
		},
	})
}
