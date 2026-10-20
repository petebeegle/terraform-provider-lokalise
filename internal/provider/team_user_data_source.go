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

	"github.com/petebeegle/lokalise-provider/internal/client"
)

var (
	_ datasource.DataSource              = &TeamUserDataSource{}
	_ datasource.DataSourceWithConfigure = &TeamUserDataSource{}
)

// NewTeamUserDataSource returns the lokalise_team_user data source.
func NewTeamUserDataSource() datasource.DataSource {
	return &TeamUserDataSource{}
}

// TeamUserDataSource looks up a Lokalise team user by email, so configs can
// reference a user_id (e.g. for lokalise_team_user_group_member) without a
// hard-coded number.
type TeamUserDataSource struct {
	client client.API
}

// TeamUserDataSourceModel is the lokalise_team_user data source model.
type TeamUserDataSourceModel struct {
	TeamID   types.Int64  `tfsdk:"team_id"`
	Email    types.String `tfsdk:"email"`
	UserID   types.Int64  `tfsdk:"user_id"`
	Fullname types.String `tfsdk:"fullname"`
	Role     types.String `tfsdk:"role"`
}

func (d *TeamUserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_user"
}

func (d *TeamUserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Lokalise team user by email (case-insensitive), returning its `user_id` for use in `lokalise_team_user_group_member.user_id`.",
		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				MarkdownDescription: "Team ID to look up the user in.",
				Required:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "User's email address. Matched case-insensitively.",
				Required:            true,
			},
			"user_id": schema.Int64Attribute{
				MarkdownDescription: "The user's Lokalise ID.",
				Computed:            true,
			},
			"fullname": schema.StringAttribute{
				MarkdownDescription: "The user's full name.",
				Computed:            true,
			},
			"role": schema.StringAttribute{
				MarkdownDescription: "The user's team role.",
				Computed:            true,
			},
		},
	}
}

func (d *TeamUserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (d *TeamUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data TeamUserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := data.TeamID.ValueInt64()
	email := data.Email.ValueString()

	users, err := d.client.ListTeamUsers(ctx, teamID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Lokalise team users", err.Error())
		return
	}

	var found *client.TeamUser
	for i := range users {
		if strings.EqualFold(users[i].Email, email) {
			found = &users[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError(
			"Team user not found",
			fmt.Sprintf("team %d has no user with email %q", teamID, email),
		)
		return
	}

	data.UserID = types.Int64Value(found.UserID)
	data.Fullname = types.StringValue(found.Fullname)
	data.Role = types.StringValue(found.Role)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
