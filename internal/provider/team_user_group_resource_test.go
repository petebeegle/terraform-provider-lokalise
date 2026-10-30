// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
	"github.com/petebeegle/terraform-provider-lokalise/internal/lokalisefake"
)

const teamUserGroupAddr = "lokalise_team_user_group.test"

// teamUserGroupFixture renders a lokalise_team_user_group block. roleOrRights
// is inserted verbatim (e.g. `role_id = 5` or `admin_rights = ["upload"]`),
// and languagesBlock is the resource's `languages` attribute, verbatim
// (including the leading `languages = { ... }` or omitted entirely).
func teamUserGroupFixture(teamID int64, name, roleOrRights, languagesBlock string) string {
	return fmt.Sprintf(`
resource "lokalise_team_user_group" "test" {
  team_id = %d
  name    = %q
  %s
  %s
}
`, teamID, name, roleOrRights, languagesBlock)
}

func teamUserGroupState(s *terraform.State) (*terraform.ResourceState, error) {
	rs, ok := s.RootModule().Resources[teamUserGroupAddr]
	if !ok {
		return nil, fmt.Errorf("%s not in state", teamUserGroupAddr)
	}
	return rs, nil
}

func TestTeamUserGroup_Lifecycle(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	languages := `
  languages = {
    reference     = ["en"]
    contributable = ["fr"]
  }
`
	config := cfg + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators", "role_id = 5", languages)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(teamUserGroupAddr, "role_id", knownvalue.Int64Exact(5)),
					stateValue(teamUserGroupAddr, "admin_rights", knownvalue.Null()),
					stateValue(teamUserGroupAddr, "name", knownvalue.StringExact("Translators")),
					stateValue(teamUserGroupAddr, "group_id", knownvalue.NotNull()),
					stateValue(teamUserGroupAddr, "team_id", knownvalue.Int64Exact(lokalisefake.DefaultTeamID)),
				},
			},
			{
				ResourceName: teamUserGroupAddr,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, err := teamUserGroupState(s)
					if err != nil {
						return "", err
					}
					return rs.Primary.Attributes["team_id"] + ":" + rs.Primary.Attributes["group_id"], nil
				},
				ImportStateVerify: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators Renamed", "role_id = 5", languages),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(teamUserGroupAddr, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(teamUserGroupAddr, "name", knownvalue.StringExact("Translators Renamed")),
				},
			},
		},
	})
}

func TestTeamUserGroup_UpdateKeepsMembers(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	languages := `
  languages = {
    contributable = ["fr"]
  }
`
	var groupID int64

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators", "role_id = 5", languages),
				Check: func(s *terraform.State) error {
					rs, err := teamUserGroupState(s)
					if err != nil {
						return err
					}
					id, err := strconv.ParseInt(rs.Primary.Attributes["group_id"], 10, 64)
					if err != nil {
						return err
					}
					groupID = id
					return nil
				},
			},
			{
				PreConfig: func() {
					fake.AddGroupMemberOutOfBand(lokalisefake.DefaultTeamID, groupID, 9001)
				},
				Config: cfg + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators Renamed", "role_id = 5", languages),
				Check: func(s *terraform.State) error {
					c := client.New(fake.URL(), "test-token", client.WithRateLimit(1000, 1000))
					g, err := c.GetGroup(context.Background(), lokalisefake.DefaultTeamID, groupID)
					if err != nil {
						return err
					}
					for _, m := range g.MemberIDs {
						if m == 9001 {
							return nil
						}
					}
					return fmt.Errorf("member 9001 missing from group after update: MemberIDs = %+v", g.MemberIDs)
				},
			},
		},
	})
}

func TestTeamUserGroup_UnknownISO(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators", "role_id = 5", `
  languages = {
    contributable = ["xx"]
  }
`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`unknown language ISO code "xx"`),
			},
		},
	})
}

func TestTeamUserGroup_RoleAndRightsConflict(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators", `role_id      = 5
  admin_rights = ["upload"]`, `
  languages = {
    contributable = ["fr"]
  }
`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?s)role_id.*admin_rights|exactly one`),
			},
		},
	})
}

func TestTeamUserGroup_RemovedOutOfBand(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators", "role_id = 5", `
  languages = {
    contributable = ["fr"]
  }
`)

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
					id, err := strconv.ParseInt(rs.Primary.Attributes["group_id"], 10, 64)
					if err != nil {
						return err
					}
					groupID = id
					return nil
				},
			},
			{
				PreConfig: func() { fake.DeleteGroupOutOfBand(lokalisefake.DefaultTeamID, groupID) },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(teamUserGroupAddr, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

func TestTeamUserGroup_ImportBadID(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	config := cfg + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators", "role_id = 5", `
  languages = {
    contributable = ["fr"]
  }
`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				Config:        config,
				ResourceName:  teamUserGroupAddr,
				ImportState:   true,
				ImportStateId: "nocolon",
				ExpectError:   regexp.MustCompile(`expected team_id:group_id`),
			},
		},
	})
}

// TestTeamUserGroup_EmptyReferenceRoundTrips asserts that an omitted or
// empty `reference` set round-trips without "inconsistent result" errors or
// a perpetual diff, even though the client sends the contributable
// languages as reference (the live API rejects an empty reference array).
func TestTeamUserGroup_EmptyReferenceRoundTrips(t *testing.T) {
	for name, languages := range map[string]string{
		"omitted reference": `languages = { contributable = ["en"] }`,
		"empty reference":   `languages = { reference = [], contributable = ["en"] }`,
	} {
		t.Run(name, func(t *testing.T) {
			fake := lokalisefake.New(t)
			config := providerConfig(fake.URL()) + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators", "role_id = 5", languages)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: config,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
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
		})
	}
}

// TestTeamUserGroup_RequiresContributableLanguage asserts that configs the
// live API would reject (no contributable language) fail at plan time.
func TestTeamUserGroup_RequiresContributableLanguage(t *testing.T) {
	for name, tc := range map[string]struct {
		languages string
		wantErr   string
	}{
		"languages omitted":   {``, `(?s)"languages" is\s+required`},
		"empty object":        {`languages = {}`, `(?s)"contributable" is\s+required`},
		"empty contributable": {`languages = { reference = ["en"], contributable = [] }`, `(?s)set must contain at least 1`},
		"both empty":          {`languages = { reference = [], contributable = [] }`, `(?s)set must contain at least 1`},
	} {
		t.Run(name, func(t *testing.T) {
			fake := lokalisefake.New(t)
			config := providerConfig(fake.URL()) + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Translators", "role_id = 5", tc.languages)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      config,
						ExpectError: regexp.MustCompile(tc.wantErr),
					},
				},
			})
		})
	}
}

// TestTeamUserGroup_AdminRightsAddedByAPI asserts that rights Lokalise adds
// on its own (branches_main_modify) don't cause "inconsistent result" errors
// or a diff, while a configured right removed out of band does.
func TestTeamUserGroup_AdminRightsAddedByAPI(t *testing.T) {
	fake := lokalisefake.New(t)
	config := providerConfig(fake.URL()) + teamUserGroupFixture(lokalisefake.DefaultTeamID, "Uploaders", `admin_rights = ["upload"]`,
		`languages = { contributable = ["en"] }`)

	var groupID int64
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(teamUserGroupAddr, "admin_rights", knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("upload"),
					})),
				},
				Check: func(s *terraform.State) error {
					rs, err := teamUserGroupState(s)
					if err != nil {
						return err
					}
					groupID, err = strconv.ParseInt(rs.Primary.Attributes["group_id"], 10, 64)
					return err
				},
			},
			{
				PreConfig: func() {
					fake.SetGroupAdminRightsOutOfBand(lokalisefake.DefaultTeamID, groupID, []string{"branches_main_modify"})
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(teamUserGroupAddr, plancheck.ResourceActionUpdate),
					},
				},
			},
		},
	})
}
