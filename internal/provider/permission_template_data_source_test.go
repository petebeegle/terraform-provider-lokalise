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

func TestPermissionTemplate_Found(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + fmt.Sprintf(`
data "lokalise_permission_template" "translator" {
  team_id = %d
  name    = "Translator"
}
`, lokalisefake.DefaultTeamID),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue("data.lokalise_permission_template.translator", "role_id", knownvalue.Int64Exact(5)),
				},
			},
		},
	})
}

func TestPermissionTemplate_NotFound(t *testing.T) {
	fake := lokalisefake.New(t)
	cfg := providerConfig(fake.URL())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg + fmt.Sprintf(`
data "lokalise_permission_template" "nope" {
  team_id = %d
  name    = "Nope"
}
`, lokalisefake.DefaultTeamID),
				ExpectError: regexp.MustCompile(`Translator`),
			},
		},
	})
}
