// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/petebeegle/terraform-provider-lokalise/internal/client"
)

var (
	_ resource.Resource                     = &ContributorResource{}
	_ resource.ResourceWithConfigure        = &ContributorResource{}
	_ resource.ResourceWithConfigValidators = &ContributorResource{}
	_ resource.ResourceWithImportState      = &ContributorResource{}
)

// NewContributorResource returns the lokalise_contributor resource.
func NewContributorResource() resource.Resource {
	return &ContributorResource{}
}

// ContributorResource manages a single contributor on a Lokalise project.
// It is non-authoritative: it manages exactly the contributor it created,
// never removing or adopting any other. The token's own user is a
// contributor on every project it can see; this resource may manage it, but
// removing it is almost certainly not what you want.
type ContributorResource struct {
	client client.API
}

// ContributorResourceModel is the lokalise_contributor state/plan model.
type ContributorResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Email       types.String `tfsdk:"email"`
	Fullname    types.String `tfsdk:"fullname"`
	RoleID      types.Int64  `tfsdk:"role_id"`
	AdminRights types.Set    `tfsdk:"admin_rights"`
	Languages   types.Set    `tfsdk:"languages"`
	UserID      types.Int64  `tfsdk:"user_id"`
}

// contributorLanguageModel is one element of the `languages` set.
type contributorLanguageModel struct {
	LangISO    types.String `tfsdk:"lang_iso"`
	IsWritable types.Bool   `tfsdk:"is_writable"`
}

func (r *ContributorResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contributor"
}

func (r *ContributorResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A contributor on a Lokalise project. This resource manages exactly the contributor it created; it never removes any other contributor. The token's own user is a contributor on every project it can see, and this resource can manage it, but removing it is almost certainly not what you want.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "`project_id:user_id`.",
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
			"email": schema.StringAttribute{
				MarkdownDescription: "Contributor's email address, matched case-insensitively against the API. Changing it (other than its case) forces a new resource.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"fullname": schema.StringAttribute{
				MarkdownDescription: "Full name to use when inviting a new user. Has no effect for an email that is already a Lokalise user. Only used on invite: it is not read back from the API afterwards, and changing it only updates state.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role_id": schema.Int64Attribute{
				MarkdownDescription: "Permission template ID to assign (see `lokalise_permission_template`). Conflicts with `admin_rights`; exactly one of the two must be set.",
				Optional:            true,
			},
			"admin_rights": schema.SetAttribute{
				MarkdownDescription: "Explicit set of admin rights to assign. Conflicts with `role_id`; exactly one of the two must be set.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"languages": schema.SetNestedAttribute{
				MarkdownDescription: "Languages this contributor may access.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"lang_iso": schema.StringAttribute{
							MarkdownDescription: "Language code, e.g. `fr`.",
							Required:            true,
						},
						"is_writable": schema.BoolAttribute{
							MarkdownDescription: "Whether the contributor may write translations in this language.",
							Required:            true,
						},
					},
				},
			},
			"user_id": schema.Int64Attribute{
				MarkdownDescription: "Lokalise user ID of the contributor.",
				Computed:            true,
			},
		},
	}
}

func (r *ContributorResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("role_id"),
			path.MatchRoot("admin_rights"),
		),
	}
}

func (r *ContributorResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *ContributorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ContributorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := plan.ProjectID.ValueString()
	email := plan.Email.ValueString()

	existing, err := r.client.ListContributors(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Lokalise project contributors", err.Error())
		return
	}
	for _, c := range existing {
		if strings.EqualFold(c.Email, email) {
			resp.Diagnostics.AddError(
				"Contributor already exists",
				fmt.Sprintf(
					"Contributor %q already exists in project %s. Import it instead: terraform import lokalise_contributor.<name> %s:%s",
					email, projectID, projectID, email,
				),
			)
			return
		}
	}

	in := client.ContributorInput{
		Email:    email,
		Fullname: plan.Fullname.ValueString(),
	}
	if !plan.RoleID.IsNull() {
		roleID := plan.RoleID.ValueInt64()
		in.RoleID = &roleID
	}
	if !plan.AdminRights.IsNull() && !plan.AdminRights.IsUnknown() {
		var rights []string
		resp.Diagnostics.Append(plan.AdminRights.ElementsAs(ctx, &rights, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		in.AdminRights = rights
	}
	langs, diags := contributorLanguagesFromSet(ctx, plan.Languages)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	in.Languages = langs

	c, err := r.client.CreateContributor(ctx, projectID, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Lokalise contributor", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyContributor(ctx, projectID, c)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ContributorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ContributorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := state.ProjectID.ValueString()

	c, err := r.client.GetContributor(ctx, projectID, state.UserID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Lokalise contributor", err.Error())
		return
	}

	resp.Diagnostics.Append(state.applyContributor(ctx, projectID, c)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ContributorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ContributorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := state.ProjectID.ValueString()

	in := client.ContributorInput{
		Email:    plan.Email.ValueString(),
		Fullname: plan.Fullname.ValueString(),
	}
	if !plan.RoleID.IsNull() {
		roleID := plan.RoleID.ValueInt64()
		in.RoleID = &roleID
	}
	if !plan.AdminRights.IsNull() && !plan.AdminRights.IsUnknown() {
		var rights []string
		resp.Diagnostics.Append(plan.AdminRights.ElementsAs(ctx, &rights, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		in.AdminRights = rights
	}
	langs, diags := contributorLanguagesFromSet(ctx, plan.Languages)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	in.Languages = langs

	c, err := r.client.UpdateContributor(ctx, projectID, state.UserID.ValueInt64(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Lokalise contributor", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyContributor(ctx, projectID, c)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ContributorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ContributorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteContributor(ctx, state.ProjectID.ValueString(), state.UserID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting Lokalise contributor", err.Error())
	}
}

func (r *ContributorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := splitID(req.ID, 2, "project_id:email")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	projectID, email := parts[0], parts[1]

	contributors, err := r.client.ListContributors(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Lokalise project contributors", err.Error())
		return
	}
	var found *client.Contributor
	for i := range contributors {
		if strings.EqualFold(contributors[i].Email, email) {
			found = &contributors[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Contributor not found", fmt.Sprintf("project %s has no contributor %q", projectID, email))
		return
	}

	var model ContributorResourceModel
	resp.Diagnostics.Append(model.applyContributor(ctx, projectID, found)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// applyContributor copies API-owned fields from c into m and recomputes id.
// email is kept when it equals c's case-insensitively, and fullname is kept
// whenever m already has one.
//
// Whether role_id or admin_rights ends up set is decided before any field
// is overwritten: if m.RoleID was already non-null (an existing resource
// configured with role_id), or if both were null (a fresh import) and c has
// a role_id, role_id wins and admin_rights is set null; otherwise
// admin_rights is populated from c and role_id is set null. This preserves
// whichever of the two the user configured instead of re-deriving it from
// the API response on every read.
func (m *ContributorResourceModel) applyContributor(ctx context.Context, projectID string, c *client.Contributor) diag.Diagnostics {
	var diags diag.Diagnostics

	useRoleID := useRoleIDOverAdminRights(m.RoleID.IsNull(), m.AdminRights.IsNull(), c.RoleID != nil)

	m.ProjectID = types.StringValue(projectID)
	// Keep the configured email when the API's differs only in case (e.g.
	// the API lowercases it), so it neither errors nor forces a replacement.
	if m.Email.IsNull() || m.Email.IsUnknown() || !strings.EqualFold(m.Email.ValueString(), c.Email) {
		m.Email = types.StringValue(c.Email)
	}
	// fullname only matters when inviting a new user; the API reports the
	// user's own name, which may differ. Read it only when there is no
	// value yet (unconfigured on create, or import).
	if m.Fullname.IsNull() || m.Fullname.IsUnknown() {
		m.Fullname = types.StringValue(c.Fullname)
	}
	m.UserID = types.Int64Value(c.UserID)
	m.ID = types.StringValue(joinID(projectID, strconv.FormatInt(c.UserID, 10)))

	if useRoleID {
		if c.RoleID != nil {
			m.RoleID = types.Int64Value(*c.RoleID)
		} else {
			m.RoleID = types.Int64Null()
		}
		m.AdminRights = types.SetNull(types.StringType)
	} else {
		m.RoleID = types.Int64Null()
		rights, d := types.SetValueFrom(ctx, types.StringType, c.AdminRights)
		diags.Append(d...)
		m.AdminRights = rights
	}

	langSet, d := contributorLanguagesToSet(ctx, c.Languages)
	diags.Append(d...)
	m.Languages = langSet

	return diags
}

// contributorLanguagesFromSet converts the `languages` set into
// []client.ContributorLanguage.
func contributorLanguagesFromSet(ctx context.Context, s types.Set) ([]client.ContributorLanguage, diag.Diagnostics) {
	var diags diag.Diagnostics
	var models []contributorLanguageModel
	diags.Append(s.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return nil, diags
	}
	out := make([]client.ContributorLanguage, 0, len(models))
	for _, m := range models {
		out = append(out, client.ContributorLanguage{
			ISO:        m.LangISO.ValueString(),
			IsWritable: m.IsWritable.ValueBool(),
		})
	}
	return out, diags
}

// contributorLanguagesToSet converts []client.ContributorLanguage into a
// `languages` set value.
func contributorLanguagesToSet(ctx context.Context, langs []client.ContributorLanguage) (types.Set, diag.Diagnostics) {
	models := make([]contributorLanguageModel, 0, len(langs))
	for _, l := range langs {
		models = append(models, contributorLanguageModel{
			LangISO:    types.StringValue(l.ISO),
			IsWritable: types.BoolValue(l.IsWritable),
		})
	}
	return types.SetValueFrom(ctx, contributorLanguageObjectType(), models)
}

// contributorLanguageObjectType is the element type of the `languages` set.
func contributorLanguageObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"lang_iso":    types.StringType,
			"is_writable": types.BoolType,
		},
	}
}
