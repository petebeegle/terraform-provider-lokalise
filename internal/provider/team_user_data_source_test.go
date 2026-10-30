// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"

	"github.com/petebeegle/terraform-provider-lokalise/internal/lokalisefake"
)

func TestTeamUser_Found(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())
	userID := fake.AddTeamUser(lokalisefake.DefaultTeamID, "Jane@Example.com")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				// Case-insensitive email matching.
				Config: cfg + fmt.Sprintf(`
data "lokalise_team_user" "jane" {
  team_id = %d
  email   = "jane@example.com"
}
`, lokalisefake.DefaultTeamID),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue("data.lokalise_team_user.jane", "user_id", knownvalue.Int64Exact(userID)),
					stateValue("data.lokalise_team_user.jane", "role", knownvalue.StringExact("member")),
				},
			},
		},
	})
}

func TestTeamUser_NotFound(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + fmt.Sprintf(`
data "lokalise_team_user" "nope" {
  team_id = %d
  email   = "nope@example.com"
}
`, lokalisefake.DefaultTeamID),
				ExpectError: regexp.MustCompile(`not found`),
			},
		},
	})
}
