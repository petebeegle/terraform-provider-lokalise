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

const teamUserGroupMemberAddr = "lokalise_team_user_group_member.test"

// teamUserGroupMemberFixture renders a lokalise_team_user_group and a
// lokalise_team_user_group_member assigning userID to it.
func teamUserGroupMemberFixture(teamID, userID int64) string {
	return fmt.Sprintf(`
resource "lokalise_team_user_group" "test" {
  team_id = %d
  name    = "Translators"
  role_id = 5

  languages = {
    contributable = ["fr"]
  }
}

resource "lokalise_team_user_group_member" "test" {
  team_id  = %d
  group_id = lokalise_team_user_group.test.group_id
  user_id  = %d
}
`, teamID, teamID, userID)
}

func teamUserGroupMemberState(s *terraform.State) (*terraform.ResourceState, error) {
	rs, ok := s.RootModule().Resources[teamUserGroupMemberAddr]
	if !ok {
		return nil, fmt.Errorf("%s not in state", teamUserGroupMemberAddr)
	}
	return rs, nil
}

func TestTeamUserGroupMember_Lifecycle(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	userID := fake.AddTeamUser(lokalisefake.DefaultTeamID, "b@example.com")
	config := cfg + teamUserGroupMemberFixture(lokalisefake.DefaultTeamID, userID)

	var groupID int64

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(teamUserGroupMemberAddr, "team_id", knownvalue.Int64Exact(lokalisefake.DefaultTeamID)),
					stateValue(teamUserGroupMemberAddr, "user_id", knownvalue.Int64Exact(userID)),
					stateValue(teamUserGroupMemberAddr, "id", knownvalue.NotNull()),
				},
				Check: func(s *terraform.State) error {
					rs, err := teamUserGroupState(s)
					if err != nil {
						return err
					}
					_, err = fmt.Sscanf(rs.Primary.Attributes["group_id"], "%d", &groupID)
					return err
				},
			},
			{
				ResourceName: teamUserGroupMemberAddr,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, err := teamUserGroupMemberState(s)
					if err != nil {
						return "", err
					}
					return rs.Primary.Attributes["team_id"] + ":" + rs.Primary.Attributes["group_id"] + ":" + rs.Primary.Attributes["user_id"], nil
				},
				ImportStateVerify: true,
			},
			{
				// A second member added out of band must survive this
				// resource's destroy: it is non-authoritative over the
				// group's other members.
				PreConfig: func() {
					fake.AddGroupMemberOutOfBand(lokalisefake.DefaultTeamID, groupID, 9002)
				},
				Config: cfg + fmt.Sprintf(`
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
					g, err := c.GetGroup(context.Background(), lokalisefake.DefaultTeamID, groupID)
					if err != nil {
						return err
					}
					if len(g.MemberIDs) != 1 || g.MemberIDs[0] != 9002 {
						return fmt.Errorf("MemberIDs after destroy = %+v, want [9002] (the configured member removed, the out-of-band one kept)", g.MemberIDs)
					}
					return nil
				},
			},
		},
	})
}

func TestTeamUserGroupMember_RemovedOutOfBand(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	userID := fake.AddTeamUser(lokalisefake.DefaultTeamID, "b@example.com")
	config := cfg + teamUserGroupMemberFixture(lokalisefake.DefaultTeamID, userID)

	var groupID int64

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					rs, err := teamUserGroupState(s)
					if err != nil {
						return err
					}
					_, err = fmt.Sscanf(rs.Primary.Attributes["group_id"], "%d", &groupID)
					return err
				},
			},
			{
				PreConfig: func() {
					fake.RemoveGroupMemberOutOfBand(lokalisefake.DefaultTeamID, groupID, userID)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(teamUserGroupMemberAddr, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

func TestTeamUserGroupMember_ImportBadID(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	userID := fake.AddTeamUser(lokalisefake.DefaultTeamID, "b@example.com")
	config := cfg + teamUserGroupMemberFixture(lokalisefake.DefaultTeamID, userID)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				Config:        config,
				ResourceName:  teamUserGroupMemberAddr,
				ImportState:   true,
				ImportStateId: "nocolon",
				ExpectError:   regexp.MustCompile(`expected team_id:group_id:user_id`),
			},
		},
	})
}

// TestTeamUserGroupMember_AlreadyMember asserts that declaring a membership
// that already exists fails with an import hint instead of silently
// adopting it (a later destroy would remove access Terraform never
// granted).
func TestTeamUserGroupMember_AlreadyMember(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	userID := fake.AddTeamUser(lokalisefake.DefaultTeamID, "b@example.com")
	full := teamUserGroupMemberFixture(lokalisefake.DefaultTeamID, userID)
	groupOnly := full[:strings.Index(full, `resource "lokalise_team_user_group_member"`)]

	var groupID int64

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
					_, err = fmt.Sscanf(rs.Primary.Attributes["group_id"], "%d", &groupID)
					return err
				},
			},
			{
				PreConfig: func() {
					fake.AddGroupMemberOutOfBand(lokalisefake.DefaultTeamID, groupID, userID)
				},
				Config:      cfg + full,
				ExpectError: regexp.MustCompile(`(?s)already exists.*terraform import lokalise_team_user_group_member\.`),
			},
			{
				Config: cfg + groupOnly,
				Check: func(s *terraform.State) error {
					for _, r := range fake.Requests() {
						if strings.HasSuffix(r, "/members/add") {
							return fmt.Errorf("existing membership was re-added: %s", r)
						}
					}
					return nil
				},
			},
		},
	})
}
