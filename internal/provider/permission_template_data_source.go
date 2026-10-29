// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
)

var (
	_ datasource.DataSource              = &PermissionTemplateDataSource{}
	_ datasource.DataSourceWithConfigure = &PermissionTemplateDataSource{}
)

// NewPermissionTemplateDataSource returns the lokalise_permission_template
// data source.
func NewPermissionTemplateDataSource() datasource.DataSource {
	return &PermissionTemplateDataSource{}
}

// PermissionTemplateDataSource looks up a team's permission template
// (Lokalise "role") by name, so configs can reference role_id by name
// instead of a hard-coded number.
type PermissionTemplateDataSource struct {
	client client.API
}

// PermissionTemplateDataSourceModel is the lokalise_permission_template
// data source model.
type PermissionTemplateDataSourceModel struct {
	TeamID types.Int64  `tfsdk:"team_id"`
	Name   types.String `tfsdk:"name"`
	RoleID types.Int64  `tfsdk:"role_id"`
}

func (d *PermissionTemplateDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_template"
}

func (d *PermissionTemplateDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Lokalise team's permission template (role) by name, returning its `role_id` for use in `lokalise_team_user_group.role_id`.",
		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				MarkdownDescription: "Team ID to look up the permission template in.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Exact permission template name, e.g. `Translator`.",
				Required:            true,
			},
			"role_id": schema.Int64Attribute{
				MarkdownDescription: "The permission template's ID.",
				Computed:            true,
			},
		},
	}
}

func (d *PermissionTemplateDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (d *PermissionTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PermissionTemplateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := data.TeamID.ValueInt64()
	name := data.Name.ValueString()

	templates, err := d.client.ListPermissionTemplates(ctx, teamID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Lokalise permission templates", err.Error())
		return
	}

	names := make([]string, 0, len(templates))
	var roleID int64
	found := false
	for _, t := range templates {
		names = append(names, t.Name)
		if t.Name == name {
			roleID = t.ID
			found = true
		}
	}
	if !found {
		resp.Diagnostics.AddError(
			"Permission template not found",
			fmt.Sprintf("team %d has no permission template named %q. Available: %s", teamID, name, strings.Join(names, ", ")),
		)
		return
	}

	data.RoleID = types.Int64Value(roleID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
