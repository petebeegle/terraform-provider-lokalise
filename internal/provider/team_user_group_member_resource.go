// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
)

var (
	_ resource.Resource                = &TeamUserGroupMemberResource{}
	_ resource.ResourceWithConfigure   = &TeamUserGroupMemberResource{}
	_ resource.ResourceWithImportState = &TeamUserGroupMemberResource{}
)

// NewTeamUserGroupMemberResource returns the lokalise_team_user_group_member
// resource.
func NewTeamUserGroupMemberResource() resource.Resource {
	return &TeamUserGroupMemberResource{}
}

// TeamUserGroupMemberResource manages a single user's membership in a team
// user group. It is non-authoritative: it manages exactly the membership it
// created, never adopting or removing any other member of the group.
type TeamUserGroupMemberResource struct {
	client client.API
}

// TeamUserGroupMemberResourceModel is the
// lokalise_team_user_group_member state/plan model.
type TeamUserGroupMemberResourceModel struct {
	ID      types.String `tfsdk:"id"`
	TeamID  types.Int64  `tfsdk:"team_id"`
	GroupID types.Int64  `tfsdk:"group_id"`
	UserID  types.Int64  `tfsdk:"user_id"`
}

func (r *TeamUserGroupMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_user_group_member"
}

func (r *TeamUserGroupMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A single user's membership in a Lokalise team user group. This resource manages exactly the membership it created; it never removes any other member of the group. Creating it for a user who is already a member fails; import the existing membership instead.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "`team_id:group_id:user_id`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_id": schema.Int64Attribute{
				MarkdownDescription: "Lokalise team ID. Changing it forces a new resource.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.Int64Attribute{
				MarkdownDescription: "Lokalise team user group ID. Changing it forces a new resource.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"user_id": schema.Int64Attribute{
				MarkdownDescription: "Lokalise user ID to add to the group. Changing it forces a new resource.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *TeamUserGroupMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *TeamUserGroupMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TeamUserGroupMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := plan.TeamID.ValueInt64()
	groupID := plan.GroupID.ValueInt64()
	userID := plan.UserID.ValueInt64()

	// Refuse to adopt an existing membership: destroying this resource
	// would then remove access Terraform never granted.
	g, err := r.client.GetGroup(ctx, teamID, groupID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Lokalise team user group", err.Error())
		return
	}
	if containsID(g.MemberIDs, userID) {
		resp.Diagnostics.AddError(
			"Group membership already exists",
			fmt.Sprintf(
				"User %d is already a member of group %d in team %d. Import it instead: terraform import lokalise_team_user_group_member.<name> %d:%d:%d",
				userID, groupID, teamID, teamID, groupID, userID,
			),
		)
		return
	}

	if err := r.client.AddGroupMembers(ctx, teamID, groupID, []int64{userID}); err != nil {
		resp.Diagnostics.AddError("Error adding Lokalise team user group member", err.Error())
		return
	}

	plan.ID = types.StringValue(joinID(strconv.FormatInt(teamID, 10), strconv.FormatInt(groupID, 10), strconv.FormatInt(userID, 10)))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamUserGroupMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TeamUserGroupMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := state.TeamID.ValueInt64()
	groupID := state.GroupID.ValueInt64()
	userID := state.UserID.ValueInt64()

	g, err := r.client.GetGroup(ctx, teamID, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Lokalise team user group", err.Error())
		return
	}

	if !containsID(g.MemberIDs, userID) {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(joinID(strconv.FormatInt(teamID, 10), strconv.FormatInt(groupID, 10), strconv.FormatInt(userID, 10)))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never invoked by Terraform: every attribute requires replace, so
// any configuration change replaces the resource instead of updating it.
// This method exists only to satisfy resource.Resource.
func (r *TeamUserGroupMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TeamUserGroupMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamUserGroupMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TeamUserGroupMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RemoveGroupMembers(ctx, state.TeamID.ValueInt64(), state.GroupID.ValueInt64(), []int64{state.UserID.ValueInt64()})
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error removing Lokalise team user group member", err.Error())
	}
}

func (r *TeamUserGroupMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := splitID(req.ID, 3, "team_id:group_id:user_id")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	teamID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("invalid team_id %q: %s", parts[0], err))
		return
	}
	groupID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("invalid group_id %q: %s", parts[1], err))
		return
	}
	userID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("invalid user_id %q: %s", parts[2], err))
		return
	}

	g, err := r.client.GetGroup(ctx, teamID, groupID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Lokalise team user group", err.Error())
		return
	}
	if !containsID(g.MemberIDs, userID) {
		resp.Diagnostics.AddError("Member not found", fmt.Sprintf("group %d has no member %d", groupID, userID))
		return
	}

	model := TeamUserGroupMemberResourceModel{
		ID:      types.StringValue(req.ID),
		TeamID:  types.Int64Value(teamID),
		GroupID: types.Int64Value(groupID),
		UserID:  types.Int64Value(userID),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// containsID reports whether v is present in ids.
func containsID(ids []int64, v int64) bool {
	for _, id := range ids {
		if id == v {
			return true
		}
	}
	return false
}
