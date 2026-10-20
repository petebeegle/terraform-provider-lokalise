// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/petebeegle/lokalise-provider/internal/client"
)

var (
	_ resource.Resource                = &ProjectLanguageResource{}
	_ resource.ResourceWithConfigure   = &ProjectLanguageResource{}
	_ resource.ResourceWithImportState = &ProjectLanguageResource{}
)

// NewProjectLanguageResource returns the lokalise_project_language resource.
func NewProjectLanguageResource() resource.Resource {
	return &ProjectLanguageResource{}
}

// ProjectLanguageResource manages a single language within a Lokalise
// project. It is non-authoritative: it never removes a language it did not
// create, and treats a project's base language as not deletable (removing
// this resource for the base language only clears Terraform state).
type ProjectLanguageResource struct {
	client client.API
}

// ProjectLanguageResourceModel is the lokalise_project_language state/plan
// model.
type ProjectLanguageResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	ProjectID          types.String `tfsdk:"project_id"`
	LangISO            types.String `tfsdk:"lang_iso"`
	LangID             types.Int64  `tfsdk:"lang_id"`
	CustomName         types.String `tfsdk:"custom_name"`
	PluralForms        types.List   `tfsdk:"plural_forms"`
	DeletionProtection types.Bool   `tfsdk:"deletion_protection"`
}

func (r *ProjectLanguageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_language"
}

func (r *ProjectLanguageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A language added to a Lokalise project. This resource manages exactly the language it created; it never removes any other language, and a project's base language cannot be deleted through this resource (see `deletion_protection`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "`project_id:lang_id`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "Lokalise project ID. Changing it forces a new resource.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"lang_iso": schema.StringAttribute{
				MarkdownDescription: "Language code, e.g. `fr`. Can be changed in place.",
				Required:            true,
			},
			"lang_id": schema.Int64Attribute{
				MarkdownDescription: "Lokalise's system-wide language ID.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"custom_name": schema.StringAttribute{
				MarkdownDescription: "Display name for the language. Defaults to Lokalise's name for `lang_iso`. When unset, the current name is kept.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"plural_forms": schema.ListAttribute{
				MarkdownDescription: "Plural forms for the language, e.g. `[\"one\", \"other\"]`. Defaults to Lokalise's forms for `lang_iso`. When unset, the current forms are kept.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"deletion_protection": schema.BoolAttribute{
				MarkdownDescription: "When `true` (the default), destroying this resource fails instead of deleting the language. Set to `false` and apply before destroying. Imported languages get `true`. Has no effect on the project's base language, which this resource never deletes.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *ProjectLanguageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *ProjectLanguageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProjectLanguageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := plan.ProjectID.ValueString()
	iso := plan.LangISO.ValueString()

	existing, err := r.client.ListLanguages(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Lokalise project languages", err.Error())
		return
	}
	for _, l := range existing {
		if l.ISO == iso {
			resp.Diagnostics.AddError(
				"Language already exists",
				fmt.Sprintf(
					"Language %q already exists in project %s. Import it instead: terraform import lokalise_project_language.<name> %s:%s",
					iso, projectID, projectID, iso,
				),
			)
			return
		}
	}

	in := client.CreateLanguageInput{ISO: iso}
	if !plan.CustomName.IsNull() && !plan.CustomName.IsUnknown() {
		in.CustomName = plan.CustomName.ValueString()
	}
	if !plan.PluralForms.IsNull() && !plan.PluralForms.IsUnknown() {
		var forms []string
		resp.Diagnostics.Append(plan.PluralForms.ElementsAs(ctx, &forms, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		in.PluralForms = forms
	}

	l, err := r.client.CreateLanguage(ctx, projectID, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Lokalise project language", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyLanguage(ctx, projectID, l)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectLanguageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProjectLanguageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := state.ProjectID.ValueString()

	l, err := r.client.GetLanguage(ctx, projectID, state.LangID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Lokalise project language", err.Error())
		return
	}

	resp.Diagnostics.Append(state.applyLanguage(ctx, projectID, l)...)
	if state.DeletionProtection.IsNull() {
		state.DeletionProtection = types.BoolValue(true)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProjectLanguageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ProjectLanguageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Send only attributes that are known and differ from state, so an
	// unconfigured custom_name/plural_forms is never overwritten with an
	// empty value. deletion_protection is Terraform-only.
	var in client.UpdateLanguageInput
	changed := false
	if !plan.LangISO.Equal(state.LangISO) {
		in.ISO = plan.LangISO.ValueString()
		changed = true
	}
	if !plan.CustomName.IsUnknown() && !plan.CustomName.IsNull() && !plan.CustomName.Equal(state.CustomName) {
		name := plan.CustomName.ValueString()
		in.Name = &name
		changed = true
	}
	if !plan.PluralForms.IsUnknown() && !plan.PluralForms.IsNull() && !plan.PluralForms.Equal(state.PluralForms) {
		var forms []string
		resp.Diagnostics.Append(plan.PluralForms.ElementsAs(ctx, &forms, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		in.PluralForms = forms
		changed = true
	}

	if !changed {
		plan.ID = state.ID
		plan.LangID = state.LangID
		if plan.CustomName.IsUnknown() {
			plan.CustomName = state.CustomName
		}
		if plan.PluralForms.IsUnknown() {
			plan.PluralForms = state.PluralForms
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	projectID := state.ProjectID.ValueString()
	l, err := r.client.UpdateLanguage(ctx, projectID, state.LangID.ValueInt64(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Lokalise project language", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyLanguage(ctx, projectID, l)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectLanguageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectLanguageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := state.ProjectID.ValueString()

	p, err := r.client.GetProject(ctx, projectID)
	if err != nil {
		if client.IsNotFound(err) {
			// The project is already gone, so this language is too.
			return
		}
		resp.Diagnostics.AddError("Error reading Lokalise project", err.Error())
		return
	}

	if state.LangISO.ValueString() == p.BaseLanguageISO {
		resp.Diagnostics.AddWarning(
			"Base language not deleted",
			"lokalise_project_language "+state.ID.ValueString()+" is project "+projectID+"'s base language, "+
				"which Lokalise does not allow deleting. It is only removed from Terraform state.",
		)
		return
	}

	if state.DeletionProtection.IsNull() || state.DeletionProtection.ValueBool() {
		resp.Diagnostics.AddError(
			"Deletion protection enabled",
			"lokalise_project_language "+state.ID.ValueString()+" has deletion_protection enabled. "+
				"Set deletion_protection = false and run terraform apply before destroying or replacing it.",
		)
		return
	}

	err = r.client.DeleteLanguage(ctx, projectID, state.LangID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting Lokalise project language", err.Error())
	}
}

func (r *ProjectLanguageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := splitID(req.ID, 2, "project_id:lang_iso")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	projectID, iso := parts[0], parts[1]

	langs, err := r.client.ListLanguages(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Lokalise project languages", err.Error())
		return
	}
	var found *client.Language
	for i := range langs {
		if langs[i].ISO == iso {
			found = &langs[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Language not found", fmt.Sprintf("project %s has no language %q", projectID, iso))
		return
	}

	var model ProjectLanguageResourceModel
	resp.Diagnostics.Append(model.applyLanguage(ctx, projectID, found)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// applyLanguage copies API-owned fields from l into m and recomputes id.
// deletion_protection is left untouched.
func (m *ProjectLanguageResourceModel) applyLanguage(ctx context.Context, projectID string, l *client.Language) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ProjectID = types.StringValue(projectID)
	m.LangID = types.Int64Value(l.ID)
	m.LangISO = types.StringValue(l.ISO)
	m.CustomName = types.StringValue(l.Name)
	m.ID = types.StringValue(joinID(projectID, strconv.FormatInt(l.ID, 10)))

	forms, d := types.ListValueFrom(ctx, types.StringType, l.PluralForms)
	diags.Append(d...)
	m.PluralForms = forms

	return diags
}
