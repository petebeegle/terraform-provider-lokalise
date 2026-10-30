// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
)

var (
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithConfigure   = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

// NewProjectResource returns the lokalise_project resource.
func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

// ProjectResource manages a Lokalise project.
type ProjectResource struct {
	client client.API
}

// ProjectResourceModel is the lokalise_project state/plan model.
type ProjectResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	TeamID             types.Int64  `tfsdk:"team_id"`
	ProjectType        types.String `tfsdk:"project_type"`
	BaseLanguageISO    types.String `tfsdk:"base_language_iso"`
	DeletionProtection types.Bool   `tfsdk:"deletion_protection"`
}

func (r *ProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Lokalise project. The base language is created with the project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Lokalise project ID.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Project name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Project description.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"team_id": schema.Int64Attribute{
				MarkdownDescription: "Team to create the project in. Defaults to the API token user's current team. Changing it forces a new project.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"project_type": schema.StringAttribute{
				MarkdownDescription: "Project type, e.g. `localization_files` or `marketing`. When omitted, Lokalise's default (`localization_files`) is used on create, and an existing or imported project keeps its type. Changing it forces a new project.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"base_language_iso": schema.StringAttribute{
				MarkdownDescription: "Language code of the project's base language, created with the project. Changing it forces a new project.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"deletion_protection": schema.BoolAttribute{
				MarkdownDescription: "When `true` (the default), destroying this resource fails instead of deleting the project. Set to `false` and apply before destroying. Imported projects get `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *ProjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := client.CreateProjectInput{
		Name:            plan.Name.ValueString(),
		Description:     plan.Description.ValueString(),
		ProjectType:     plan.ProjectType.ValueString(),
		BaseLanguageISO: plan.BaseLanguageISO.ValueString(),
	}
	if !plan.TeamID.IsNull() && !plan.TeamID.IsUnknown() {
		in.TeamID = plan.TeamID.ValueInt64()
	}

	p, err := r.client.CreateProject(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Lokalise project", err.Error())
		return
	}

	plan.applyProject(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.GetProject(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Lokalise project", err.Error())
		return
	}

	state.applyProject(p)
	if state.DeletionProtection.IsNull() {
		state.DeletionProtection = types.BoolValue(true)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// deletion_protection is Terraform-only; skip the API when only it changed.
	if plan.Name.Equal(state.Name) && plan.Description.Equal(state.Description) {
		plan.ID = state.ID
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	p, err := r.client.UpdateProject(ctx, state.ID.ValueString(), client.UpdateProjectInput{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating Lokalise project", err.Error())
		return
	}

	plan.applyProject(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.DeletionProtection.IsNull() || state.DeletionProtection.ValueBool() {
		resp.Diagnostics.AddError(
			"Deletion protection enabled",
			"lokalise_project "+state.ID.ValueString()+" has deletion_protection enabled. "+
				"Set deletion_protection = false and run terraform apply before destroying or replacing it.",
		)
		return
	}

	err := r.client.DeleteProject(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting Lokalise project", err.Error())
	}
}

func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyProject copies API-owned fields from p into m. deletion_protection is
// left untouched.
func (m *ProjectResourceModel) applyProject(p *client.Project) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Description = types.StringValue(p.Description)
	m.TeamID = types.Int64Value(p.TeamID)
	m.ProjectType = types.StringValue(p.ProjectType)
	m.BaseLanguageISO = types.StringValue(p.BaseLanguageISO)
}
