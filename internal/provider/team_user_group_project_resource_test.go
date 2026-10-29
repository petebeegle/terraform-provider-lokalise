// Copyright IBM Corp. 2021, 2026
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

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
	"github.com/petebeegle/terraform-provider-lokalise/internal/lokalisefake"
)

const teamUserGroupProjectAddr = "lokalise_team_user_group_project.test"

// teamUserGroupProjectFixture renders a project ("tf-p", base language
// "en"), a lokalise_team_user_group and a lokalise_team_user_group_project
// assigning the project to the group.
func teamUserGroupProjectFixture(teamID int64) string {
	return projectConfig("tf-p", "en", "deletion_protection = false") + fmt.Sprintf(`
resource "lokalise_team_user_group" "test" {
  team_id = %d
  name    = "Translators"
  role_id = 5

  languages = {
    contributable = ["fr"]
  }
}

resource "lokalise_team_user_group_project" "test" {
  team_id    = %d
  group_id   = lokalise_team_user_group.test.group_id
  project_id = lokalise_project.test.id
}
`, teamID, teamID)
}

func teamUserGroupProjectState(s *terraform.State) (*terraform.ResourceState, error) {
	rs, ok := s.RootModule().Resources[teamUserGroupProjectAddr]
	if !ok {
		return nil, fmt.Errorf("%s not in state", teamUserGroupProjectAddr)
	}
	return rs, nil
}

func TestTeamUserGroupProject_Lifecycle(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + teamUserGroupProjectFixture(lokalisefake.DefaultTeamID)

	var groupID int64
	var projectID string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(teamUserGroupProjectAddr, "team_id", knownvalue.Int64Exact(lokalisefake.DefaultTeamID)),
					stateValue(teamUserGroupProjectAddr, "id", knownvalue.NotNull()),
				},
				Check: func(s *terraform.State) error {
					grs, err := teamUserGroupState(s)
					if err != nil {
						return err
					}
					if _, err := fmt.Sscanf(grs.Primary.Attributes["group_id"], "%d", &groupID); err != nil {
						return err
					}
					prs, err := projectIDFromState(s)
					if err != nil {
						return err
					}
					projectID = prs
					return nil
				},
			},
			{
				ResourceName: teamUserGroupProjectAddr,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, err := teamUserGroupProjectState(s)
					if err != nil {
						return "", err
					}
					return rs.Primary.Attributes["team_id"] + ":" + rs.Primary.Attributes["group_id"] + ":" + rs.Primary.Attributes["project_id"], nil
				},
				ImportStateVerify: true,
			},
			{
				// Destroying the project assignment must leave both the
				// group and the project in place.
				Config: cfg + projectConfig("tf-p", "en", "deletion_protection = false") + fmt.Sprintf(`
resource "lokalise_team_user_group" "test" {
  team_id = %d
  name    = "Translators"
  role_id = 5

  languages = {
    contributable = ["fr"]
  }
}
`, lokalisefake.DefaultTeamID),
				Check: func(s *terraform.State) error {
					c := client.New(fake.URL(), "test-token", client.WithRateLimit(1000, 1000))
					if _, err := c.GetProject(context.Background(), projectID); err != nil {
						return fmt.Errorf("project removed after destroying the assignment: %w", err)
					}
					g, err := c.GetGroup(context.Background(), lokalisefake.DefaultTeamID, groupID)
					if err != nil {
						return fmt.Errorf("group removed after destroying the assignment: %w", err)
					}
					if len(g.ProjectIDs) != 0 {
						return fmt.Errorf("ProjectIDs after destroy = %+v, want empty", g.ProjectIDs)
					}
					return nil
				},
			},
		},
	})
}

func TestTeamUserGroupProject_RemovedOutOfBand(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + teamUserGroupProjectFixture(lokalisefake.DefaultTeamID)

	var groupID int64
	var projectID string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					grs, err := teamUserGroupState(s)
					if err != nil {
						return err
					}
					if _, err := fmt.Sscanf(grs.Primary.Attributes["group_id"], "%d", &groupID); err != nil {
						return err
					}
					prs, err := projectIDFromState(s)
					if err != nil {
						return err
					}
					projectID = prs
					return nil
				},
			},
			{
				PreConfig: func() {
					fake.RemoveGroupProjectOutOfBand(lokalisefake.DefaultTeamID, groupID, projectID)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(teamUserGroupProjectAddr, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

func TestTeamUserGroupProject_ImportBadID(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + teamUserGroupProjectFixture(lokalisefake.DefaultTeamID)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				Config:        config,
				ResourceName:  teamUserGroupProjectAddr,
				ImportState:   true,
				ImportStateId: "nocolon",
				ExpectError:   regexp.MustCompile(`expected team_id:group_id:project_id`),
			},
		},
	})
}

// TestTeamUserGroupProject_AlreadyAssigned asserts that declaring a project
// assignment that already exists fails with an import hint instead of
// silently adopting it.
func TestTeamUserGroupProject_AlreadyAssigned(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	full := teamUserGroupProjectFixture(lokalisefake.DefaultTeamID)
	groupOnly := full[:strings.Index(full, `resource "lokalise_team_user_group_project"`)]

	var groupID int64
	var projectID string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + groupOnly,
				Check: func(s *terraform.State) error {
					rs, err := teamUserGroupState(s)
					if err != nil {
						return err
					}
					if _, err := fmt.Sscanf(rs.Primary.Attributes["group_id"], "%d", &groupID); err != nil {
						return err
					}
					projectID, err = projectIDFromState(s)
					return err
				},
			},
			{
				PreConfig: func() {
					fake.AddGroupProjectOutOfBand(lokalisefake.DefaultTeamID, groupID, projectID)
				},
				Config:      cfg + full,
				ExpectError: regexp.MustCompile(`(?s)already exists.*terraform import lokalise_team_user_group_project\.`),
			},
			{
				Config: cfg + groupOnly,
				Check: func(s *terraform.State) error {
					for _, r := range fake.Requests() {
						if strings.HasSuffix(r, "/projects/add") {
							return fmt.Errorf("existing assignment was re-added: %s", r)
						}
					}
					return nil
				},
			},
		},
	})
}
