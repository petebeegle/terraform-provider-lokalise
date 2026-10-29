// Copyright IBM Corp. 2021, 2026
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
	_ resource.Resource                = &TeamUserGroupProjectResource{}
	_ resource.ResourceWithConfigure   = &TeamUserGroupProjectResource{}
	_ resource.ResourceWithImportState = &TeamUserGroupProjectResource{}
)

// NewTeamUserGroupProjectResource returns the
// lokalise_team_user_group_project resource.
func NewTeamUserGroupProjectResource() resource.Resource {
	return &TeamUserGroupProjectResource{}
}

// TeamUserGroupProjectResource manages a single project's assignment to a
// team user group. It is non-authoritative: it manages exactly the
// assignment it created, never adopting or removing any other project
// assigned to the group.
type TeamUserGroupProjectResource struct {
	client client.API
}

// TeamUserGroupProjectResourceModel is the
// lokalise_team_user_group_project state/plan model.
type TeamUserGroupProjectResourceModel struct {
	ID        types.String `tfsdk:"id"`
	TeamID    types.Int64  `tfsdk:"team_id"`
	GroupID   types.Int64  `tfsdk:"group_id"`
	ProjectID types.String `tfsdk:"project_id"`
}

func (r *TeamUserGroupProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_user_group_project"
}

func (r *TeamUserGroupProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A single project's assignment to a Lokalise team user group. This resource manages exactly the assignment it created; it never removes any other project assigned to the group. Creating it for a project that is already assigned fails; import the existing assignment instead.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "`team_id:group_id:project_id`.",
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
			"project_id": schema.StringAttribute{
				MarkdownDescription: "Lokalise project ID to assign to the group. Changing it forces a new resource.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *TeamUserGroupProjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *TeamUserGroupProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TeamUserGroupProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := plan.TeamID.ValueInt64()
	groupID := plan.GroupID.ValueInt64()
	projectID := plan.ProjectID.ValueString()

	// Refuse to adopt an existing assignment: destroying this resource
	// would then remove access Terraform never granted.
	g, err := r.client.GetGroup(ctx, teamID, groupID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Lokalise team user group", err.Error())
		return
	}
	if containsProjectID(g.ProjectIDs, projectID) {
		resp.Diagnostics.AddError(
			"Group project assignment already exists",
			fmt.Sprintf(
				"Project %s is already assigned to group %d in team %d. Import it instead: terraform import lokalise_team_user_group_project.<name> %d:%d:%s",
				projectID, groupID, teamID, teamID, groupID, projectID,
			),
		)
		return
	}

	if err := r.client.AddGroupProjects(ctx, teamID, groupID, []string{projectID}); err != nil {
		resp.Diagnostics.AddError("Error adding Lokalise team user group project", err.Error())
		return
	}

	plan.ID = types.StringValue(joinID(strconv.FormatInt(teamID, 10), strconv.FormatInt(groupID, 10), projectID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamUserGroupProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TeamUserGroupProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	teamID := state.TeamID.ValueInt64()
	groupID := state.GroupID.ValueInt64()
	projectID := state.ProjectID.ValueString()

	g, err := r.client.GetGroup(ctx, teamID, groupID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Lokalise team user group", err.Error())
		return
	}

	if !containsProjectID(g.ProjectIDs, projectID) {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(joinID(strconv.FormatInt(teamID, 10), strconv.FormatInt(groupID, 10), projectID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never invoked by Terraform: every attribute requires replace, so
// any configuration change replaces the resource instead of updating it.
// This method exists only to satisfy resource.Resource.
func (r *TeamUserGroupProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TeamUserGroupProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TeamUserGroupProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TeamUserGroupProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RemoveGroupProjects(ctx, state.TeamID.ValueInt64(), state.GroupID.ValueInt64(), []string{state.ProjectID.ValueString()})
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error removing Lokalise team user group project", err.Error())
	}
}

func (r *TeamUserGroupProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := splitID(req.ID, 3, "team_id:group_id:project_id")
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
	projectID := parts[2]

	g, err := r.client.GetGroup(ctx, teamID, groupID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Lokalise team user group", err.Error())
		return
	}
	if !containsProjectID(g.ProjectIDs, projectID) {
		resp.Diagnostics.AddError("Project not found", fmt.Sprintf("group %d has no project %q", groupID, projectID))
		return
	}

	model := TeamUserGroupProjectResourceModel{
		ID:        types.StringValue(req.ID),
		TeamID:    types.Int64Value(teamID),
		GroupID:   types.Int64Value(groupID),
		ProjectID: types.StringValue(projectID),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// containsProjectID reports whether v is present in ids.
func containsProjectID(ids []string, v string) bool {
	for _, id := range ids {
		if id == v {
			return true
		}
	}
	return false
}
